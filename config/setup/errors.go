package setup

import (
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strconv"

	"github.com/hashicorp/go-syslog"
	"github.com/mholt/caddy/middleware"
	"github.com/mholt/caddy/middleware/errors"
)

// Errors configures a new gzip middleware instance.
func Errors(c *Controller) (middleware.Middleware, error) {
	handler, err := errorsParse(c)
	if err != nil {
		return nil, err
	}

	// Open the log file for writing when the server starts
	c.Startup = append(c.Startup, func() error {
		var err error
		var writer io.Writer

		if handler.LogFile == "stdout" {
			writer = os.Stdout
		} else if handler.LogFile == "stderr" {
			writer = os.Stderr
		} else if handler.LogFile == "syslog" {
			writer, err = gsyslog.NewLogger(gsyslog.LOG_ERR, "LOCAL0", "caddy")
			if err != nil {
				return err
			}
		} else if handler.LogFile != "" {
			var file *os.File
			file, err = os.OpenFile(handler.LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				return err
			}
			if handler.LogRoller != nil {
				file.Close()

				handler.LogRoller.Filename = handler.LogFile

				writer = handler.LogRoller.GetLogWriter()
			} else {
				writer = file
			}
		}

		handler.Log = log.New(writer, "", 0)
		return nil
	})

	return func(next middleware.Handler) middleware.Handler {
		handler.Next = next
		return handler
	}, nil
}

func errorsParse(c *Controller) (*errors.ErrorHandler, error) {
	// Very important that we make a pointer because the Startup
	// function that opens the log file must have access to the
	// same instance of the handler, not a copy.
	handler := &errors.ErrorHandler{ErrorPages: make(map[int]string)}

	optionalBlock := func() (bool, error) {
		var hadBlock bool

		for c.NextBlock() {
			hadBlock = true

			what := c.Val()
			if !c.NextArg() {
				return hadBlock, c.ArgErr()
			}
			where := c.Val()

			if what == "log" {
				handler.LogFile = where
				if c.NextArg() {
					if c.Val() == "{" {
						c.IncrNest()
						logRoller, err := parseRoller(c)
						if err != nil {
							return hadBlock, err
						}
						handler.LogRoller = logRoller
					}
				}
			} else {
				// Error page; ensure it exists
				where = path.Join(c.Root, where)
				f, err := os.Open(where)
				if err != nil {
					fmt.Println("Warning: Unable to open error page '" + where + "': " + err.Error())
				}
				f.Close()

				whatInt, err := strconv.Atoi(what)
				if err != nil {
					return hadBlock, c.Err("Expecting a numeric status code, got '" + what + "'")
				}
				handler.ErrorPages[whatInt] = where
			}
		}
		return hadBlock, nil
	}

	for c.Next() {
		// weird hack to avoid having the handler values overwritten.
		if c.Val() == "}" {
			continue
		}
		// Configuration may be in a block
		hadBlock, err := optionalBlock()
		if err != nil {
			return handler, err
		}

		// Otherwise, the only argument would be an error log file name
		if !hadBlock {
			if c.NextArg() {
				handler.LogFile = c.Val()
			} else {
				handler.LogFile = errors.DefaultLogFilename
			}
		}
	}

	return handler, nil
}
