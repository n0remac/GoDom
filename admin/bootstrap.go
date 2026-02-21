package admin

import (
	"errors"
	"flag"
	"fmt"

	"github.com/n0remac/GoDom/auth"
)

type Bootstrapper interface {
	EnsureAdmin(email, password string) (bool, error)
}

func HandleCLI(bootstrapper Bootstrapper, args []string, programName string) (handled bool, message string, err error) {
	if len(args) == 0 {
		return false, "", nil
	}
	if args[0] != "admin" {
		return false, "", nil
	}
	if len(args) < 2 || args[1] != "create" {
		return true, "", fmt.Errorf("usage: %s admin create --email <email> --password <password>", programName)
	}

	fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
	email := fs.String("email", "", "admin email")
	password := fs.String("password", "", "admin password")
	if err := fs.Parse(args[2:]); err != nil {
		return true, "", err
	}
	if *email == "" || *password == "" {
		return true, "", errors.New("email and password are required")
	}

	created, err := bootstrapper.EnsureAdmin(*email, *password)
	if err != nil {
		return true, "", err
	}
	if created {
		return true, fmt.Sprintf("created admin user for %s", *email), nil
	}
	return true, fmt.Sprintf("set admin role for existing user %s", *email), nil
}

func MissingAdminWarning(users auth.UserRepo, programName string) (string, error) {
	adminCount, err := users.CountByRole(auth.RoleAdmin)
	if err != nil {
		return "", err
	}
	if adminCount == 0 {
		return fmt.Sprintf("WARNING: no admin user configured. Run `%s admin create --email <email> --password <password>`", programName), nil
	}
	return "", nil
}
