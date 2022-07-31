package uitemplates

// ActiveUserParams holds information about the active user.
type ActiveUserParams struct {
	// LoggedIn is true if the current user is logged in.
	LoggedIn bool

	// Email is the user's email.
	Email string
}
