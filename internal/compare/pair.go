package compare

import (
	"fmt"
	"sort"

	"github.com/4ugane/kubectl-env-diff/internal/config"
	"github.com/4ugane/kubectl-env-diff/internal/model"
)

// Options controls pairing. Exactly one naming strategy applies, resolved in
// the order documented in the spec: explicit pair, then single name, then whole
// namespace.
type Options struct {
	Name      string
	FromName  string
	ToName    string
	Normalize config.NormalizeRules
}

// Pair is one comparison slot. A nil side means the resource is absent there,
// which is itself one of the most valuable findings.
type Pair struct {
	Kind     model.Kind
	Name     string
	FromName string
	ToName   string
	From     *model.Workload
	To       *model.Workload
	FromCM   *model.ConfigMap
	ToCM     *model.ConfigMap
}

// Pairs matches resources across two snapshots.
func Pairs(from, to model.Snapshot, opts Options) ([]Pair, error) {
	if opts.FromName != "" || opts.ToName != "" {
		return pairExplicit(from, to, opts)
	}

	slots := map[key]*Pair{}

	add := func(snap model.Snapshot, isFrom bool) error {
		// seen tracks which original name claimed each normalized key on this
		// side, so a collision can name both offenders.
		seen := map[key]string{}

		record := func(k key, original string) (*Pair, error) {
			if prev, ok := seen[k]; ok {
				return nil, fmt.Errorf(
					"%s names %q and %q both normalize to %q; refusing to guess which pairs with the other cluster",
					k.Kind, prev, original, k.Name)
			}
			seen[k] = original
			p, ok := slots[k]
			if !ok {
				p = &Pair{Kind: k.Kind, Name: k.Name}
				slots[k] = p
			}
			if isFrom {
				p.FromName = original
			} else {
				p.ToName = original
			}
			return p, nil
		}

		for i := range snap.Workloads {
			w := &snap.Workloads[i]
			norm, err := NormalizeName(w.Name, opts.Normalize)
			if err != nil {
				return err
			}
			if opts.Name != "" && norm != opts.Name {
				continue
			}
			p, err := record(key{w.Kind, norm}, w.Name)
			if err != nil {
				return err
			}
			if isFrom {
				p.From = w
			} else {
				p.To = w
			}
		}

		for i := range snap.ConfigMaps {
			cm := &snap.ConfigMaps[i]
			norm, err := NormalizeName(cm.Name, opts.Normalize)
			if err != nil {
				return err
			}
			if opts.Name != "" && norm != opts.Name {
				continue
			}
			p, err := record(key{model.KindConfigMap, norm}, cm.Name)
			if err != nil {
				return err
			}
			if isFrom {
				p.FromCM = cm
			} else {
				p.ToCM = cm
			}
		}
		return nil
	}

	if err := add(from, true); err != nil {
		return nil, err
	}
	if err := add(to, false); err != nil {
		return nil, err
	}
	return sortPairs(slots), nil
}

// pairExplicit handles --from-name/--to-name. Normalization is skipped: the user
// has stated the mapping directly.
func pairExplicit(from, to model.Snapshot, opts Options) ([]Pair, error) {
	slots := map[key]*Pair{}
	found := false

	for i := range from.Workloads {
		if w := &from.Workloads[i]; w.Name == opts.FromName {
			k := key{w.Kind, opts.FromName}
			slots[k] = &Pair{Kind: w.Kind, Name: opts.FromName, FromName: w.Name, From: w}
			found = true
		}
	}
	for i := range from.ConfigMaps {
		if cm := &from.ConfigMaps[i]; cm.Name == opts.FromName {
			k := key{model.KindConfigMap, opts.FromName}
			slots[k] = &Pair{Kind: model.KindConfigMap, Name: opts.FromName, FromName: cm.Name, FromCM: cm}
			found = true
		}
	}

	attach := func(k key, set func(*Pair)) {
		if p, ok := slots[k]; ok {
			set(p)
			p.ToName = opts.ToName
			return
		}
		// Present on the to side only.
		p := &Pair{Kind: k.Kind, Name: opts.FromName, ToName: opts.ToName}
		set(p)
		slots[k] = p
		found = true
	}

	for i := range to.Workloads {
		if w := &to.Workloads[i]; w.Name == opts.ToName {
			attach(key{w.Kind, opts.FromName}, func(p *Pair) { p.To = w })
		}
	}
	for i := range to.ConfigMaps {
		if cm := &to.ConfigMaps[i]; cm.Name == opts.ToName {
			attach(key{model.KindConfigMap, opts.FromName}, func(p *Pair) { p.ToCM = cm })
		}
	}

	if !found {
		return nil, fmt.Errorf(
			"neither %q (in %s/%s) nor %q (in %s/%s) exists; check the names",
			opts.FromName, from.Context, from.Namespace,
			opts.ToName, to.Context, to.Namespace)
	}
	return sortPairs(slots), nil
}

// sortPairs imposes a total order so runs are reproducible and golden tests are
// stable.
func sortPairs(slots map[key]*Pair) []Pair {
	out := make([]Pair, 0, len(slots))
	for _, p := range slots {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out
}
