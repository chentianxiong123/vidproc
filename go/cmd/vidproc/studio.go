package main

import (
	"errors"
	"flag"
	"fmt"

	"vidproc/internal/server"
)

// cmdStudio 启动 Web Studio(htmx GUI,对齐 HomeCast server 模式)。
func cmdStudio(args []string) error {
	fs := flag.NewFlagSet("studio", flag.ContinueOnError)
	port := fs.String("port", "", "listen port (default 8398, or VIDPROC_PORT)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("studio: unexpected argument " + fs.Arg(0))
	}

	addr := ""
	if *port != "" {
		addr = "127.0.0.1:" + *port
	}
	fmt.Println("vidproc studio — htmx Web Studio (Ctrl+C to stop)")
	fmt.Println("open http://127.0.0.1:8398 in your browser")
	return server.ListenAndServe(addr)
}