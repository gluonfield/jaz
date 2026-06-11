package skillsprompt

import (
	"strings"
	"testing"
)

func TestRenderExactShapeAndEscaping(t *testing.T) {
	prompt, err := Render(Data{Skills: []Skill{
		{Name: "deploy", Description: "ship & verify <fast>", Location: "/skills/deploy/SKILL.md"},
		{Name: "review", Description: "strict review", Location: "/skills/review/SKILL.md"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := `## Skills
Use a listed skill when named or when its description matches the task. Users may reference a skill inline as $name in their messages. Read its SKILL.md first; resolve relative paths from that file; load extras only as needed.

<available_skills>
  <skill>
    <name>deploy</name>
    <description>ship &amp; verify &lt;fast&gt;</description>
    <location>/skills/deploy/SKILL.md</location>
  </skill>
  <skill>
    <name>review</name>
    <description>strict review</description>
    <location>/skills/review/SKILL.md</location>
  </skill>
</available_skills>`
	if prompt != want {
		t.Fatalf("skills prompt drifted from the exact contract.\nGOT:\n%s\nWANT:\n%s", prompt, want)
	}
	if strings.Count(prompt, "<available_skills>") != 1 || strings.Count(prompt, "</available_skills>") != 1 {
		t.Fatalf("skills wrapper must appear exactly once:\n%s", prompt)
	}
}
