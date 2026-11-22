package toolsearch

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/kljensen/snowball/english"
	unidecode "github.com/mozillazg/go-unidecode"
	"github.com/rivo/uniseg"
)

var toolSearchStopwords = newToolSearchStopwords()

type Document struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Index struct {
	documents    []indexedDocument
	averageTerms float64
}

type indexedDocument struct {
	name       string
	termCounts map[string]int
	termTotal  int
}

func New(documents []Document) Index {
	index := Index{documents: make([]indexedDocument, 0, len(documents))}
	for _, document := range documents {
		parts := []string{document.Name, strings.ReplaceAll(document.Name, "_", " "), document.Description}
		appendSchemaSearchText(document.Parameters, &parts)
		tokens := tokenizeToolSearch(strings.Join(parts, " "))
		index.documents = append(index.documents, indexedDocument{document.Name, termCounts(tokens), len(tokens)})
		index.averageTerms += float64(len(tokens))
	}
	if len(documents) > 0 {
		index.averageTerms /= float64(len(documents))
	}
	return index
}

func (i Index) Rank(query string) []int {
	queryTerms := tokenizeToolSearch(query)
	if len(queryTerms) == 0 || len(i.documents) == 0 {
		return nil
	}
	documentFrequency := map[string]int{}
	for _, term := range uniqueTerms(queryTerms) {
		for _, document := range i.documents {
			if document.termCounts[term] > 0 {
				documentFrequency[term]++
			}
		}
	}
	type match struct {
		position int
		score    float64
	}
	var matches []match
	for position, document := range i.documents {
		score := i.bm25Score(document, queryTerms, documentFrequency)
		if score > 0 {
			matches = append(matches, match{position, score})
		}
	}
	sort.SliceStable(matches, func(a, b int) bool {
		if matches[a].score == matches[b].score {
			return i.documents[matches[a].position].name < i.documents[matches[b].position].name
		}
		return matches[a].score > matches[b].score
	})
	ranked := make([]int, len(matches))
	for index, result := range matches {
		ranked[index] = result.position
	}
	return ranked
}

func (i Index) bm25Score(entry indexedDocument, queryTerms []string, documentFrequency map[string]int) float64 {
	const (
		k1 = 1.2
		b  = 0.75
	)
	if entry.termTotal == 0 || i.averageTerms == 0 {
		return 0
	}
	n := float64(len(i.documents))
	lengthFactor := k1 * (1 - b + b*float64(entry.termTotal)/i.averageTerms)
	var score float64
	for _, term := range queryTerms {
		tf := float64(entry.termCounts[term])
		if tf == 0 {
			continue
		}
		df := float64(documentFrequency[term])
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		score += idf * (tf * (k1 + 1)) / (tf + lengthFactor)
	}
	return score
}

func appendSchemaSearchText(value any, parts *[]string) {
	switch schema := value.(type) {
	case map[string]any:
		if description, ok := schema["description"].(string); ok {
			*parts = append(*parts, description)
		}
		if properties, ok := schema["properties"].(map[string]any); ok {
			for name, child := range properties {
				*parts = append(*parts, name)
				appendSchemaSearchText(child, parts)
			}
		}
		appendSchemaSearchText(schema["items"], parts)
		appendSchemaSearchText(schema["anyOf"], parts)
	case []any:
		for _, child := range schema {
			appendSchemaSearchText(child, parts)
		}
	}
}

func tokenizeToolSearch(text string) []string {
	text = strings.ToLower(unidecode.Unidecode(text))
	tokens := []string{}
	for state := -1; text != ""; {
		word, rest, newState := uniseg.FirstWordInString(text, state)
		state = newState
		if rest == text {
			break
		}
		text = rest
		if !toolSearchWordToken(word) || toolSearchStopwords[word] {
			continue
		}
		if stem := english.Stem(word, false); stem != "" {
			tokens = append(tokens, stem)
		}
	}
	return tokens
}

func toolSearchWordToken(token string) bool {
	for _, r := range token {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func termCounts(tokens []string) map[string]int {
	counts := map[string]int{}
	for _, token := range tokens {
		counts[token]++
	}
	return counts
}

func uniqueTerms(tokens []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

func newToolSearchStopwords() map[string]bool {
	words := strings.Fields(`i me my myself we our ours ourselves you you're you've you'll you'd your yours yourself yourselves he him his himself she she's her hers herself it it's its itself they them their theirs themselves what which who whom this that that'll these those am is are was were be been being have has had having do does did doing a an the and but if or because as until while of at by for with about against between into through during before after above below to from up down in out on off over under again further then once here there when where why how all any both each few more most other some such no nor not only own same so than too very s t can will just don don't should should've now d ll m o re ve y ain aren aren't couldn couldn't didn didn't doesn doesn't hadn hadn't hasn hasn't haven haven't isn isn't ma mightn mightn't mustn mustn't needn needn't shan shan't shouldn shouldn't wasn wasn't weren weren't won won't wouldn wouldn't`)
	stopwords := make(map[string]bool, len(words))
	for _, word := range words {
		stopwords[word] = true
	}
	return stopwords
}
