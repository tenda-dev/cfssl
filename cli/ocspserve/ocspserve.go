// Package ocspserve implements the ocspserve function.
package ocspserve

import (
	"errors"
	"net"
	"net/http"
	"strconv"

	"github.com/cloudflare/cfssl/cli"
	"github.com/cloudflare/cfssl/log"
	"github.com/cloudflare/cfssl/ocsp"
)

// Usage text of 'cfssl serve'
var ocspServerUsageText = `cfssl ocspserve -- set up an HTTP server that handles OCSP requests from either a file or directly from a database or from redis service (see RFC 5019)

  Usage of ocspserve:
          cfssl ocspserve [-address address] [-port port] [-responses file] [-db-config db-config] [-redis host:port]

  Flags:
  `

// Flags used by 'cfssl serve'
var ocspServerFlags = []string{"address", "port", "responses", "db-config", "redis"}

// ocspServerMain is the command line entry point to the OCSP responder.
// It sets up a new HTTP server that responds to OCSP requests.
func ocspServerMain(args []string, c cli.Config) error {
	var src ocsp.Source
	// serve doesn't support arguments.
	if len(args) > 0 {
		return errors.New("argument is provided but not defined; please refer to the usage by flag -h")
	}

	if c.Redis != "" {
		s, err := ocsp.NewSourceFromRedis(c.Redis)
		if err != nil {
			return errors.New("unable to connect to redis service")
		}
		src = s
	} else if c.Responses != "" {
		s, err := ocsp.NewSourceFromFile(c.Responses)
		if err != nil {
			return errors.New("unable to read response file")
		}
		src = s
	} else if c.DBConfigFile != "" {
		s, err := ocsp.NewSourceFromDB(c.DBConfigFile)
		if err != nil {
			return errors.New("unable to read configuration file")
		}
		src = s
	} else {
		return errors.New(
			"no response file nor db/redis config provided, please set the one of these using either -responses or -db-config or -redis flags",
		)
	}

	log.Info("Registering OCSP responder handler")
	http.Handle(c.Path, ocsp.NewResponder(src, nil))

	addr := net.JoinHostPort(c.Address, strconv.Itoa(c.Port))
	log.Info("Now listening on ", addr)
	return http.ListenAndServe(addr, nil)
}

// Command assembles the definition of Command 'ocspserve'
var Command = &cli.Command{UsageText: ocspServerUsageText, Flags: ocspServerFlags, Main: ocspServerMain}
