package auth

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/n0remac/GoDom/html"
	. "github.com/n0remac/GoDom/websocket"
)

type AuthApp struct {
	Users    UserRepo
	Sessions SessionRepo
}

func NewAuthApp() *AuthApp {
	return &AuthApp{
		Users:    NewInMemoryUserStore(),
		Sessions: NewSessionStore(),
	}
}

// Auth mounts auth routes using default in-memory stores (backwards compatible)
func Auth(mux *http.ServeMux, websocketRegistry *CommandRegistry) *AuthApp {
	app := NewAuthApp()
	mountAuthHandlers(mux, app)
	return app
}

// AuthWithStores mounts auth routes using provided user/session repositories.
func AuthWithStores(mux *http.ServeMux, websocketRegistry *CommandRegistry, users UserRepo, sessions SessionRepo) *AuthApp {
	app := &AuthApp{Users: users, Sessions: sessions}
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

func (a *AuthApp) RegisterPage() *Node {
	return DefaultLayout(
		Div(Class("min-h-screen flex items-center justify-center"),
			Div(Class("card p-6 w-96"),
				H2(Text("Register")),
				Form(Method("POST"), Action("/register"),
					Div(Text("Email")),
					Input(Type("email"), Name("email"), Class("input input-bordered w-full mb-2")),
					Div(Text("Password")),
					Input(Type("password"), Name("password"), Class("input input-bordered w-full mb-4")),
					Button(Type("submit"), Class("btn btn-primary w-full"), Text("Register")),
				),
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
			email := r.FormValue("email")
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
			ServeNode(a.RegisterPage())(w, r)
			return
		case http.MethodPost:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			email := r.FormValue("email")
			password := r.FormValue("password")
			if email == "" || password == "" {
				http.Error(w, "missing fields", http.StatusBadRequest)
				return
			}
			if _, err := a.Users.CreateUser(email, password); err != nil {
				http.Error(w, fmt.Sprintf("error: %v", err), http.StatusBadRequest)
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
		sid, ok := getSessionIDFromRequest(r)
		if !ok {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		sess, ok := a.Sessions.Get(sid)
		if !ok {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		user, err := a.Users.GetByID(sess.UserID)
		if err != nil {
			http.Error(w, "user not found", http.StatusUnauthorized)
			return
		}
		// Return a small fragment
		ServeNode(Div(Class("p-4"), Text(fmt.Sprintf("Logged in as %s", user.Email))))(w, r)
	}
}
