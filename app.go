package grove

// App is the central runtime object for a Grove service. It holds private state
// and exposes public methods for registering capabilities during module
// registration. All fields are private to keep the public API stable and explicit.
type App struct {
	name string
}

// Name returns the service name, derived from Module.Name().
func (a *App) Name() string {
	return a.name
}

// newApp creates an App with the given name and applies the provided options.
func newApp(name string, opts ...Option) *App {
	a := &App{
		name: name,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
