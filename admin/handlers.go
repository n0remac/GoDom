package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/n0remac/GoDom/auth"
	. "github.com/n0remac/GoDom/html"
)

type App struct {
	Auth *auth.AuthApp
}

func Mount(mux *http.ServeMux, authApp *auth.AuthApp) *App {
	app := &App{Auth: authApp}
	mux.HandleFunc("/admin", app.pageHandler())
	mux.HandleFunc("/admin/registration-mode", app.registrationModeHandler())
	mux.HandleFunc("/admin/invites/create", app.inviteCreateHandler())
	mux.HandleFunc("/admin/users/role", app.userRoleHandler())
	return app
}

func (a *App) Page(adminUser *auth.User) *Node {
	mode, _ := a.Auth.Registration.GetRegistrationMode()
	invites, _ := a.Auth.Invites.ListInvites()
	users, _ := a.Auth.Users.ListUsers()

	return DefaultLayout(
		Div(Class("min-h-screen p-6 space-y-6"),
			Div(Class("card p-6 bg-base-200"),
				H2(Text("Admin")),
				P(Text(fmt.Sprintf("Signed in as %s", adminUser.Email))),
			),
			a.registrationPanel(mode, ""),
			a.invitesPanel(invites, ""),
			a.usersPanel(users, ""),
		),
	)
}

func (a *App) pageHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		user, ok := a.requireAdmin(w, r)
		if !ok {
			return
		}
		ServeNode(a.Page(user))(w, r)
	}
}

func (a *App) registrationModeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := a.requireAdmin(w, r); !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		mode := auth.RegistrationMode(r.FormValue("mode"))
		message := "Registration mode updated"
		if err := a.Auth.Registration.SetRegistrationMode(mode); err != nil {
			message = fmt.Sprintf("Error: %v", err)
		}
		currentMode, _ := a.Auth.Registration.GetRegistrationMode()
		ServeNode(a.registrationPanel(currentMode, message))(w, r)
	}
}

func (a *App) inviteCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		adminUser, ok := a.requireAdmin(w, r)
		if !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		ttlHours := 24
		if raw := r.FormValue("ttl_hours"); raw != "" {
			hours, err := strconv.Atoi(raw)
			if err != nil || hours < 0 {
				invites, _ := a.Auth.Invites.ListInvites()
				ServeNode(a.invitesPanel(invites, "TTL must be a whole number greater than or equal to 0"))(w, r)
				return
			}
			ttlHours = hours
		}

		invite, err := a.Auth.Invites.CreateInvite(adminUser.ID, time.Duration(ttlHours)*time.Hour)
		if err != nil {
			invites, _ := a.Auth.Invites.ListInvites()
			ServeNode(a.invitesPanel(invites, fmt.Sprintf("Error: %v", err)))(w, r)
			return
		}

		invites, _ := a.Auth.Invites.ListInvites()
		ServeNode(a.invitesPanel(invites, fmt.Sprintf("Invite created: /register?invite=%s", invite.Token)))(w, r)
	}
}

func (a *App) userRoleHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, ok := a.requireAdmin(w, r); !ok {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		userID := r.FormValue("user_id")
		role := r.FormValue("role")
		users, _ := a.Auth.Users.ListUsers()
		if userID == "" || role == "" {
			ServeNode(a.usersPanel(users, "User and role are required"))(w, r)
			return
		}

		target, err := a.Auth.Users.GetByID(userID)
		if err != nil {
			ServeNode(a.usersPanel(users, fmt.Sprintf("Error: %v", err)))(w, r)
			return
		}

		if target.Role == auth.RoleAdmin && role != auth.RoleAdmin {
			admins, err := a.Auth.Users.CountByRole(auth.RoleAdmin)
			if err != nil {
				ServeNode(a.usersPanel(users, "Error checking admin count"))(w, r)
				return
			}
			if admins <= 1 {
				ServeNode(a.usersPanel(users, "Cannot demote the last admin"))(w, r)
				return
			}
		}

		if err := a.Auth.Users.UpdateRole(userID, role); err != nil {
			users, _ = a.Auth.Users.ListUsers()
			ServeNode(a.usersPanel(users, fmt.Sprintf("Error: %v", err)))(w, r)
			return
		}

		users, _ = a.Auth.Users.ListUsers()
		ServeNode(a.usersPanel(users, "User role updated"))(w, r)
	}
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	user, ok := a.Auth.CurrentUser(r)
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return nil, false
	}
	if user.Role != auth.RoleAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return nil, false
	}
	return user, true
}

func (a *App) registrationPanel(mode auth.RegistrationMode, message string) *Node {
	return Div(
		Id("admin-registration"),
		Class("card p-6 bg-base-200 space-y-4"),
		H3(Text("Registration Mode")),
		If(message != "", Div(Class("alert"), Text(message)), Nil()),
		Form(
			Method("POST"),
			HxPost("/admin/registration-mode"),
			HxTarget("#admin-registration"),
			HxSwap("outerHTML"),
			Div(Class("flex gap-2 items-end flex-wrap"),
				Div(
					Label(For("registration-mode"), Text("Mode")),
					Select(
						Id("registration-mode"),
						Name("mode"),
						Class("select select-bordered"),
						modeOption(auth.RegistrationOpen, mode),
						modeOption(auth.RegistrationInviteOnly, mode),
						modeOption(auth.RegistrationClosed, mode),
					),
				),
				Button(Type("submit"), Class("btn btn-primary"), Text("Save")),
			),
		),
	)
}

func modeOption(candidate, selected auth.RegistrationMode) *Node {
	return Option(
		Value(string(candidate)),
		If(candidate == selected, Attr("selected", "selected"), Nil()),
		Text(string(candidate)),
	)
}

func (a *App) invitesPanel(invites []*auth.Invite, message string) *Node {
	rows := make([]*Node, 0, len(invites))
	now := time.Now()
	for _, invite := range invites {
		status := "unused"
		if invite.IsUsed() {
			status = "used"
		} else if invite.IsExpired(now) {
			status = "expired"
		}
		link := "/register?invite=" + invite.Token
		rows = append(rows, Tr(
			Td(Class("font-mono text-xs"), Text(invite.Token)),
			Td(A(Href(link), Text(link))),
			Td(Text(status)),
			Td(Text(formatTime(invite.CreatedAt))),
			Td(Text(formatOptionalTime(invite.ExpiresAt))),
		))
	}

	table := Div(Text("No invites have been created yet."))
	if len(rows) > 0 {
		table = Div(Class("overflow-x-auto"),
			Table(Class("table table-zebra"),
				Thead(Tr(
					Th(Text("Token")),
					Th(Text("Link")),
					Th(Text("Status")),
					Th(Text("Created")),
					Th(Text("Expires")),
				)),
				Tbody(Ch(rows)),
			),
		)
	}

	return Div(
		Id("admin-invites"),
		Class("card p-6 bg-base-200 space-y-4"),
		H3(Text("Invites")),
		If(message != "", Div(Class("alert"), Text(message)), Nil()),
		Form(
			Method("POST"),
			HxPost("/admin/invites/create"),
			HxTarget("#admin-invites"),
			HxSwap("outerHTML"),
			Div(Class("flex gap-2 items-end flex-wrap"),
				Div(
					Label(For("ttl-hours"), Text("TTL hours (0 means no expiry)")),
					Input(
						Id("ttl-hours"),
						Type("number"),
						Name("ttl_hours"),
						Value("24"),
						Attr("min", "0"),
						Class("input input-bordered"),
					),
				),
				Button(Type("submit"), Class("btn btn-primary"), Text("Create Invite")),
			),
		),
		table,
	)
}

func (a *App) usersPanel(users []*auth.User, message string) *Node {
	rows := make([]*Node, 0, len(users))
	for _, user := range users {
		role := user.Role
		if role == "" {
			role = auth.RoleMember
		}
		rows = append(rows, Tr(
			Td(Text(user.Email)),
			Td(Text(role)),
			Td(Text(formatTime(user.CreatedAt))),
			Td(
				Form(
					Method("POST"),
					HxPost("/admin/users/role"),
					HxTarget("#admin-users"),
					HxSwap("outerHTML"),
					Class("flex gap-2 items-center"),
					Input(Type("hidden"), Name("user_id"), Value(user.ID)),
					Select(
						Name("role"),
						Class("select select-bordered select-sm"),
						Option(Value(auth.RoleMember), If(role == auth.RoleMember, Attr("selected", "selected"), Nil()), Text(auth.RoleMember)),
						Option(Value(auth.RoleAdmin), If(role == auth.RoleAdmin, Attr("selected", "selected"), Nil()), Text(auth.RoleAdmin)),
					),
					Button(Type("submit"), Class("btn btn-sm"), Text("Update")),
				),
			),
		))
	}

	return Div(
		Id("admin-users"),
		Class("card p-6 bg-base-200 space-y-4"),
		H3(Text("Users")),
		If(message != "", Div(Class("alert"), Text(message)), Nil()),
		Div(Class("overflow-x-auto"),
			Table(Class("table table-zebra"),
				Thead(Tr(
					Th(Text("Email")),
					Th(Text("Role")),
					Th(Text("Created")),
					Th(Text("Actions")),
				)),
				Tbody(Ch(rows)),
			),
		),
	)
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return formatTime(*t)
}

func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04 UTC")
}
