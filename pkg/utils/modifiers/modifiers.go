package modifiers

// ProcessMods processes modifiers for functions using the fluent pattern
func ProcessMods[M any](cfg *M, mods []func(*M)) {
	for _, mod := range mods {
		mod(cfg)
	}
}

// MakeMods creates and processes modifiers
func MakeMods[M any](mods []func(*M)) *M {
	m := new(M)
	ProcessMods(m, mods)
	return m
}
