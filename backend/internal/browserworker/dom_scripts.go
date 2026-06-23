package browserworker

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func normalizeBrowserURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}
	if strings.HasPrefix(strings.ToLower(raw), "about:") {
		return raw, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported browser URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("browser URL requires a host")
	}
	return u.String(), nil
}

func jsString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func resolvePointScript(selector string) string {
	return elementResolverJS() + `
	(function(q){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	const r = el.getBoundingClientRect();
	return {found:true, x:r.left + r.width / 2, y:r.top + r.height / 2, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(selector)) + ");"
}

func focusScript(selector string) string {
	return elementResolverJS() + `
	(function(q){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	el.focus();
	return {found:true, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(selector)) + ");"
}

func setValueScript(selector, text string, selectOnly bool) string {
	return elementResolverJS() + `
	(function(q, value, selectOnly){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	el.focus();
	let changed = true;
	if (el.tagName === "SELECT") {
	  const needle = jazNorm(value);
	  const option = Array.from(el.options).find(o => jazNorm(o.value) === needle || jazNorm(o.textContent) === needle);
	  if (!option) changed = false; else el.value = option.value;
	} else if (selectOnly) {
	  changed = false;
	} else if (el.isContentEditable) {
	  el.textContent = value;
	} else {
	  el.value = value;
	}
	if (changed) {
	  el.dispatchEvent(new Event("input", {bubbles:true}));
	  el.dispatchEvent(new Event("change", {bubbles:true}));
	}
	return {found:true, changed, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(selector)) + "," + jsString(text) + "," + strconv.FormatBool(selectOnly) + ");"
}

func elementResolverJS() string {
	return `
	function jazNorm(s){ return String(s || "").replace(/\s+/g, " ").trim().toLowerCase(); }
	function jazLabel(el){
	  const raw = el.getAttribute("aria-label") || el.getAttribute("name") || el.getAttribute("placeholder") || el.getAttribute("title") || el.innerText || el.textContent || el.value || el.tagName;
	  return el.tagName.toLowerCase() + " " + JSON.stringify(String(raw || "").replace(/\s+/g, " ").trim().slice(0, 120));
	}
	function jazFindElement(q){
	  q = String(q || "").trim();
	  if (!q) return document.activeElement;
	  try {
	    const selected = document.querySelector(q);
	    if (selected) return selected;
	  } catch (_) {}
	  const needle = jazNorm(q);
	  const els = Array.from(document.querySelectorAll("a,button,input,textarea,select,label,[role],[aria-label],[title],[placeholder],[contenteditable=true]"));
	  return els.find(el => jazNorm(el.getAttribute("aria-label") || el.getAttribute("name") || el.getAttribute("placeholder") || el.getAttribute("title") || el.innerText || el.textContent || el.value) === needle)
	    || els.find(el => jazNorm(el.getAttribute("aria-label") || el.getAttribute("name") || el.getAttribute("placeholder") || el.getAttribute("title") || el.innerText || el.textContent || el.value).includes(needle));
	}
	`
}
