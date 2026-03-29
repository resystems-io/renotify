// Package broker manages NATS connectivity for the daemon: the
// embedded server, auth configuration, and client connections.
package broker

import (
	"fmt"

	"github.com/nats-io/nats-server/v2/server"
)

// BuildAuthConfig constructs NATS server options with two-account
// auth. The daemon account (internal token) has full access. The
// mobile account (pairing token) has scoped ACLs. If pairingToken
// is empty, only the daemon account is configured.
//
// See docs/analysis-nats-transport-design.md Section 7.
func BuildAuthConfig(username, internalToken, pairingToken string) []*server.User {
	prefix := fmt.Sprintf("resystems.renotify.%s", username)

	daemonUser := &server.User{
		Username: "daemon",
		Password: internalToken,
		Permissions: &server.Permissions{
			Publish: &server.SubjectPermission{
				Allow: []string{
					prefix + ".>",
					"$JS.API.>",
				},
			},
			Subscribe: &server.SubjectPermission{
				Allow: []string{
					prefix + ".>",
					"$JS.API.>",
					"_INBOX.>",
				},
			},
			Response: &server.ResponsePermission{
				MaxMsgs: server.DEFAULT_ALLOW_RESPONSE_MAX_MSGS,
				Expires: server.DEFAULT_ALLOW_RESPONSE_EXPIRATION,
			},
		},
	}

	users := []*server.User{daemonUser}

	if pairingToken != "" {
		mobileUser := &server.User{
			Username: "mobile",
			Password: pairingToken,
			Permissions: &server.Permissions{
				Publish: &server.SubjectPermission{
					Allow: []string{
						prefix + ".flow.*.response",
						prefix + ".flow.*.interject",
						prefix + ".svc.*",
						"$JS.ACK.>",
						"$JS.FC.>",
					},
				},
				Subscribe: &server.SubjectPermission{
					Allow: []string{
						prefix + ".>",
						"_INBOX.>",
					},
				},
				Response: &server.ResponsePermission{
					MaxMsgs: server.DEFAULT_ALLOW_RESPONSE_MAX_MSGS,
					Expires: server.DEFAULT_ALLOW_RESPONSE_EXPIRATION,
				},
			},
		}
		users = append(users, mobileUser)
	}

	return users
}
