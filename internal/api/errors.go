package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Every error the API answers with is {"error": code, "message": text}.
// The code is one of the keys below and does not change between releases;
// a client branches on it. The message is English for people reading a
// log and may be reworded. Nothing else is put in it: not a server path,
// not a decoder's or database's own wording.
var errorText = map[string]string{
	"bad_request":        "The request is not valid.",
	"bad_json":           "The request body is not valid JSON or has fields of the wrong type.",
	"json_required":      "The request body must be JSON (Content-Type: application/json).",
	"confirm_required":   "The request must say what to delete and carry confirm: true.",
	"bad_dir":            "dir is in or out.",
	"bad_exporter":       "The device address is not an IP address.",
	"bad_user":           "A user name cannot be empty, longer than 64 characters, or contain spaces, colons or a leading #.",
	"too_short":          "The new password is too short.",
	"default":            "The new password cannot be the default password.",
	"wrong_old":          "The current password is wrong.",
	"wrong_login":        "Wrong user name or password.",
	"too_many_attempts":  "Too many failed sign-ins from this address; try again in a minute.",
	"login":              "Sign in first.",
	"change_password":    "The default password must be changed first.",
	"cross_origin":       "The request came from another site.",
	"not_admin":          "Only the administrator may do this.",
	"fixed":              "The passwords were given at start and cannot be changed here.",
	"exists":             "A user with this name already exists.",
	"no_user":            "There is no such user.",
	"self":               "You cannot delete yourself.",
	"last_user":          "The last user cannot be deleted.",
	"not_available":      "This is not available in this edition.",
	"not_found":          "There is no such API route.",
	"method_not_allowed": "The route does not take this method; Allow lists the ones it takes.",
	"file":               "A file could not be read or written; the reason is in the service log.",
	"database_memory":    "The database ran out of memory; try a shorter range.",
	"database_storage":   "The database is busy or failing; try again shortly.",
}

// apiError answers with one of the codes above.
func apiError(w http.ResponseWriter, status int, code string) {
	msg, ok := errorText[code]
	if !ok {
		code, msg = "bad_request", errorText["bad_request"]
	}
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

// requestError is a request the API refuses with its own explanation, such
// as a filter it does not know: answered 400 with bad_request and that
// explanation as the message.
type requestError struct{ msg string }

func (e requestError) Error() string { return e.msg }

// isJSONError reports whether err came from decoding a request body.
func isJSONError(err error) bool {
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	var me *http.MaxBytesError
	return errors.As(err, &se) || errors.As(err, &te) || errors.As(err, &me) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}
