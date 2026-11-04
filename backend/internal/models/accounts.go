package models

import "time"

// Account is a signed-in person.
type Account struct {
	ID              string       `json:"id"`
	Provider        AuthProvider `json:"provider"`
	ProviderSubject string       `json:"-"`
	Email           string       `json:"email"`
	DisplayName     string       `json:"display_name"`
	Role            AccountRole  `json:"role"`
	LastSignInAt    *time.Time   `json:"last_sign_in_at"`
	CreatedAt       time.Time    `json:"created_at"`
}

// Session is what a client receives after signing in.
type Session struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	Account   Account   `json:"account"`
}
