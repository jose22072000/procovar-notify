package channels

import "testing"

func TestHTMLToText(t *testing.T) {
	cases := map[string]string{
		"<p>Hola <b>mundo</b></p>":       "Hola mundo",
		"line1<br>line2":                 "line1\nline2",
		"a &amp; b &lt;c&gt;":            "a & b <c>",
		"<div>uno</div><div>dos</div>":   "uno\ndos",
		"  <span>  espacios   </span>  ": "espacios",
	}
	for in, want := range cases {
		if got := htmlToText(in); got != want {
			t.Errorf("htmlToText(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestMessageTextFallsBackToSubject(t *testing.T) {
	if got := messageText(RenderedMessage{Subject: "asunto", HTMLBody: "<p></p>"}); got != "asunto" {
		t.Errorf("messageText = %q, quería 'asunto'", got)
	}
}
