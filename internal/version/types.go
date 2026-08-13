package version

// release is one parsed vMAJOR.MINOR.PATCH semantic version.
type release struct {
	major uint64
	minor uint64
	patch uint64
}
