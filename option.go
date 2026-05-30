package grove

// Option configures an App during construction. Options are applied in order
// before module registration runs.
type Option func(*App)
