package compare

import (
	"strings"

	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// multilineThreshold is the line count above which a value gets a line-level
// diff instead of whole-value replacement.
const multilineThreshold = 2

// ConfigMap diffs one paired ConfigMap key by key.
func ConfigMap(p Pair) []model.Difference {
	base := func(path string, typ model.DiffType, from, to string) model.Difference {
		return model.Difference{
			Kind: model.KindConfigMap, Name: p.Name, FromName: p.FromName, ToName: p.ToName,
			Path: path, Type: typ, From: from, To: to,
		}
	}

	switch {
	case p.FromCM == nil && p.ToCM == nil:
		return nil
	case p.ToCM == nil:
		return []model.Difference{base("", model.MissingInTo, p.FromName, "")}
	case p.FromCM == nil:
		return []model.Difference{base("", model.MissingInFrom, "", p.ToName)}
	}

	var diffs []model.Difference
	for _, k := range unionKeys(p.FromCM.Data, p.ToCM.Data) {
		fv, okF := p.FromCM.Data[k]
		tv, okT := p.ToCM.Data[k]
		ffp, fMasked := p.FromCM.Fingerprints[k]
		tfp, tMasked := p.ToCM.Fingerprints[k]
		masked := fMasked || tMasked
		path := "data." + k
		switch {
		case !okT:
			d := base(path, model.MissingInTo, fv, "")
			d.Redacted = masked
			diffs = append(diffs, d)
		case !okF:
			d := base(path, model.MissingInFrom, "", tv)
			d.Redacted = masked
			diffs = append(diffs, d)
		default:
			// Masked entries compare by fingerprint, never by their shared
			// placeholder text, so a differing credential in a ConfigMap is
			// still detected as different rather than silently vanishing.
			equal := fv == tv
			if masked {
				equal = ffp == tfp
			}
			if equal {
				continue
			}
			d := base(path, model.ValueChanged, fv, tv)
			d.Redacted = masked
			if !masked && (isMultiline(fv) || isMultiline(tv)) {
				d.Multiline = true
				d.From, d.To = changedLines(fv, tv)
			}
			diffs = append(diffs, d)
		}
	}

	if p.FromCM.Immutable != p.ToCM.Immutable {
		diffs = append(diffs, base("immutable", model.ValueChanged,
			boolStr(p.FromCM.Immutable), boolStr(p.ToCM.Immutable)))
	}
	return diffs
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func isMultiline(s string) bool {
	return strings.Count(strings.TrimRight(s, "\n"), "\n")+1 > multilineThreshold
}

// changedLines reduces two multi-line values to only the lines that differ,
// each prefixed with its 1-based line number. Reporting an entire embedded
// application.yml on both sides would bury the one line that actually changed.
func changedLines(from, to string) (string, string) {
	fl := strings.Split(strings.TrimRight(from, "\n"), "\n")
	tl := strings.Split(strings.TrimRight(to, "\n"), "\n")

	var fOut, tOut []string
	max := len(fl)
	if len(tl) > max {
		max = len(tl)
	}
	for i := 0; i < max; i++ {
		var f, t string
		if i < len(fl) {
			f = fl[i]
		}
		if i < len(tl) {
			t = tl[i]
		}
		if f == t {
			continue
		}
		if i < len(fl) {
			fOut = append(fOut, lineRef(i+1, f))
		}
		if i < len(tl) {
			tOut = append(tOut, lineRef(i+1, t))
		}
	}
	return strings.Join(fOut, "\n"), strings.Join(tOut, "\n")
}

func lineRef(n int, line string) string {
	return "L" + itoa(n) + ": " + line
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
