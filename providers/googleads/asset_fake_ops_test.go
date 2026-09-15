package googleads_test

import "strings"

func (f *assetFake) operations() []mutateOp {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]mutateOp, 0, len(f.ops))
	for _, raw := range f.ops {
		parts := strings.SplitN(raw, ":", 2)
		op := mutateOp{collection: parts[0]}
		if len(parts) == 2 {
			op.kind = parts[1]
		}
		out = append(out, op)
	}
	return out
}
