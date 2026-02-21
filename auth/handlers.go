package auth

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	. "github.com/n0remac/GoDom/html"
	. "github.com/n0remac/GoDom/websocket"
)

type AuthApp struct {
	Users        UserRepo
	Sessions     SessionRepo
	Registration RegistrationRepo
	Invites      InviteRepo
}

func NewAuthApp() *AuthApp {
	return &AuthApp{
		Users:        NewInMemoryUserStore(),
		Sessions:     NewSessionStore(),
		Registration: NewInMemoryRegistrationStore(),
		Invites:      NewInMemoryInviteStore(),
	}
}

// Auth mounts auth routes using default in-memory stores (backwards compatible).
func Auth(mux *http.ServeMux, websocketRegistry *CommandRegistry) *AuthApp {
	app := NewAuthApp()
	mountAuthHandlers(mux, app)
	return app
}

// AuthWithStores mounts auth routes using provided repositories.
func AuthWithStores(mux *http.ServeMux, websocketRegistry *CommandRegistry, users UserRepo, sessions SessionRepo) *AuthApp {
	registration, ok := users.(RegistrationRepo)
	if !ok {
		registration = NewInMemoryRegistrationStore()
	}
	invites, ok := users.(InviteRepo)
	if !ok {
		invites = NewInMemoryInviteStore()
	}
	app := &AuthApp{
		Users:        users,
		Sessions:     sessions,
		Registration: registration,
		Invites:      invites,
	}
	mountAuthHandlers(mux, app)
	return app
}

func mountAuthHandlers(mux *http.ServeMux, app *AuthApp) {
	mux.HandleFunc("/login", app.loginHandler())
	mux.HandleFunc("/register", app.registerHandler())
	mux.HandleFunc("/logout", app.logoutHandler())
	mux.HandleFunc("/me", app.meHandler())
}

func (a *AuthApp) LoginPage() *Node {
	return DefaultLayout(
		Attrs(map[string]string{
			"class":      "flex flex-col items-center min-h-screen",
			"data-theme": "dark",
		}),
		Div(Class("min-h-screen flex items-center justify-center"),
			Div(Class("card p-6 w-96"),
				H2(Text("Login")),
				Form(Method("POST"), Action("/login"),
					Div(Text("Email")),
					Input(Type("email"), Name("email"), Class("input input-bordered w-full mb-2")),
					Div(Text("Password")),
					Input(Type("password"), Name("password"), Class("input input-bordered w-full mb-4")),
					Button(Type("submit"), Class("btn btn-primary w-full"), Text("Login")),
				),
				Div(Class("mt-4 text-center"), A(Href("/register"), Text("Register"))),
			),
		),
	)
}

func (a *AuthApp) RegisterPage(mode RegistrationMode, inviteToken, errorText string) *Node {
	notice := Nil()
	if mode == RegistrationClosed {
		notice = Div(
			Class("alert alert-warning mb-4"),
			Text("Registration is currently closed."),
		)
	}
	if errorText != "" {
		notice = Div(
			Class("alert alert-error mb-4"),
			Text(errorText),
		)
	}

	registerForm := Div(
		Class("text-center text-sm text-base-content/70"),
		Text("Registration is not available right now."),
	)
	if mode != RegistrationClosed {
		inviteField := Nil()
		if mode == RegistrationInviteOnly {
			inviteField = Div(
				Div(Text("Invite Token")),
				Input(
					Type("text"),
					Name("invite_token"),
					Value(inviteToken),
					Class("input input-bordered w-full mb-2"),
					Placeholder("Paste invite token"),
				),
			)
		}
		registerForm = Form(Method("POST"), Action("/register"),
			Div(Text("Email")),
			Input(Type("email"), Name("email"), Class("input input-bordered w-full mb-2")),
			Div(Text("Password")),
			Input(Type("password"), Name("password"), Class("input input-bordered w-full mb-2")),
			inviteField,
			Button(Type("submit"), Class("btn btn-primary w-full"), Text("Register")),
		)
	}

	return DefaultLayout(
		Attrs(map[string]string{
			"class":      "flex flex-col items-center min-h-screen",
			"data-theme": "dark",
		}),
		Div(Class("min-h-screen flex items-center justify-center"),
			Div(Class("card p-6 w-96"),
				H2(Text("Register")),
				notice,
				registerForm,
				Div(Class("mt-4 text-center"), A(Href("/login"), Text("Login"))),
			),
		),
	)
}

func (a *AuthApp) loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			ServeNode(a.LoginPage())(w, r)
			return
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			email := normalizeEmail(r.FormValue("email"))
			password := r.FormValue("password")
			if email == "" || password == "" {
				http.Error(w, "missing fields", http.StatusBadRequest)
				return
			}
			if err := a.Users.VerifyPassword(email, password); err != nil {
				http.Error(w, "invalid credentials", http.StatusUnauthorized)
				return
			}
			user, _ := a.Users.GetByEmail(email)
			sess, err := a.Sessions.Create(user.ID, 24*time.Hour)
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			setSessionCookie(w, sess.ID, 24*time.Hour)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *AuthApp) registerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			mode, err := a.Registration.GetRegistrationMode()
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			ServeNode(a.RegisterPage(mode, r.URL.Query().Get("invite"), ""))(w, r)
			return
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			mode, err := a.Registration.GetRegistrationMode()
			if err != nil {
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}

			email := normalizeEmail(r.FormValue("email"))
			password := r.FormValue("password")
			if email == "" || password == "" {
				ServeNode(a.RegisterPage(mode, r.FormValue("invite_token"), "Email and password are required"))(w, r)
				return
			}

			switch mode {
			case RegistrationClosed:
				ServeNode(a.RegisterPage(mode, "", "Registration is currently closed"))(w, r)
				return
			case RegistrationOpen:
				if _, err := a.Users.CreateUser(email, password); err != nil {
					ServeNode(a.RegisterPage(mode, "", fmt.Sprintf("error: %v", err)))(w, r)
					return
				}
			case RegistrationInviteOnly:
				token := r.FormValue("invite_token")
				if token == "" {
					ServeNode(a.RegisterPage(mode, token, "Invite token is required"))(w, r)
					return
				}
				user, err := a.Users.CreateUser(email, password)
				if err != nil {
					ServeNode(a.RegisterPage(mode, token, fmt.Sprintf("error: %v", err)))(w, r)
					return
				}
				if err := a.Invites.ConsumeInvite(token, user.ID); err != nil {
					_ = a.Users.DeleteUser(user.ID)
					ServeNode(a.RegisterPage(mode, token, inviteErrorText(err)))(w, r)
					return
				}
			default:
				ServeNode(a.RegisterPage(mode, "", "Registration mode is not configured correctly"))(w, r)
				return
			}

			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (a *AuthApp) logoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		sid, ok := getSessionIDFromRequest(r)
		if ok {
			a.Sessions.Delete(sid)
		}
		clearSessionCookie(w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (a *AuthApp) meHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := a.CurrentUser(r)
		if !ok {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}
		ServeNode(Div(Class("p-4"), Text(fmt.Sprintf("Logged in as %s (%s)", user.Email, user.Role))))(w, r)
	}
}

func (a *AuthApp) CurrentUser(r *http.Request) (*User, bool) {
	sid, ok := getSessionIDFromRequest(r)
	if !ok {
		return nil, false
	}
	sess, ok := a.Sessions.Get(sid)
	if !ok {
		return nil, false
	}
	user, err := a.Users.GetByID(sess.UserID)
	if err != nil {
		return nil, false
	}
	if user.Role == "" {
		user.Role = RoleMember
	}
	return user, true
}

func (a *AuthApp) IsAdmin(r *http.Request) bool {
	user, ok := a.CurrentUser(r)
	return ok && user.Role == RoleAdmin
}

func inviteErrorText(err error) string {
	switch {
	case errors.Is(err, ErrInviteNotFound):
		return "Invite token was not found"
	case errors.Is(err, ErrInviteUsed):
		return "Invite token has already been used"
	case errors.Is(err, ErrInviteExpired):
		return "Invite token has expired"
	default:
		return fmt.Sprintf("invite error: %v", err)
	}
}
