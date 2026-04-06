// Package broker manages NATS connectivity for the daemon: the
// embedded server, auth configuration, and client connections.
package broker

import (
	"fmt"

	"github.com/nats-io/nats-server/v2/server"

	"go.resystems.io/renotify/cli/internal/state"
)

// BuildAuthConfig constructs NATS server auth with the daemon
// account and one account per paired mobile device. Each device
// has scoped ACLs. If no devices are paired, only the daemon
// account is configured.
//
// See docs/analysis-nats-transport-design.md Section 7.
func BuildAuthConfig(
	username, internalToken string,
	devices []state.PairedDevice,
) []*server.User {
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

	for _, d := range devices {
		mobileUser := &server.User{
			Username: state.NatsUsername(d.DeviceID),
			Password: d.Token,
			Permissions: &server.Permissions{
				Publish: &server.SubjectPermission{
					Allow: []string{
						prefix + ".flow.*.response",
						prefix + ".flow.*.interject",
						prefix + ".svc.*",
						"$JS.ACK.>",
						"$JS.FC.>",
						"$JS.API.CONSUMER.INFO.>",
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
