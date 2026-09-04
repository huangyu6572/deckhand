package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"localaihub/internal/platform"
	"localaihub/internal/wire"
)

type globals struct {
	JSON        bool
	JSONL       bool
	Quiet       bool
	Timeout     string
	AllowPublic bool
	Detach      bool
	Sensitive   bool
}

func addGlobal(cmd *cobra.Command, g *globals) {
	cmd.Flags().BoolVar(&g.JSON, "json", false, "stdout is one JSON object")
	cmd.Flags().BoolVar(&g.JSONL, "jsonl", false, "stdout is JSONL events")
	cmd.Flags().BoolVar(&g.Quiet, "quiet", false, "less text")
	cmd.Flags().StringVar(&g.Timeout, "timeout", "", "duration such as 30s")
	cmd.Flags().BoolVar(&g.AllowPublic, "allow-public", false, "allow non-intranet targets")
	cmd.Flags().BoolVar(&g.Detach, "detach", false, "return job id immediately")
	cmd.Flags().BoolVar(&g.Sensitive, "sensitive", false, "do not persist command body")
}

func timeoutMS(g *globals) int64 {
	if g.Timeout == "" {
		return 0
	}
	d, err := time.ParseDuration(g.Timeout)
	if err != nil {
		return -1
	}
	return d.Milliseconds()
}

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "hub",
		Short:         "Local AI remote engineering hub",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectFlagsBeforeVerb(os.Args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(2)
			}
			if err := rejectPasswordArgv(os.Args[1:]); err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(2)
			}
			return nil
		},
	}
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		fmt.Fprintln(os.Stderr, "INVALID_ARGUMENT: global flags must follow the verb")
		os.Exit(2)
		return err
	})

	var g globals
	var scriptFile, runShell string

	run := &cobra.Command{
		Use:   "run [flags] <target> [--script-file <path>] [--shell bash|powershell|pwsh|raw] [-- <command...>]",
		Short: "Run a remote SSH command",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if timeoutMS(&g) < 0 {
				return exitErr(wire.E("INVALID_ARGUMENT", "invalid --timeout"), &g)
			}
			target, command, warn, err := prepareRunCommand(args, scriptFile, runShell)
			if err != nil {
				return exitErr(err, &g)
			}
			if warn != "" && !g.Quiet {
				fmt.Fprintln(os.Stderr, "hub:", warn)
			}
			params := map[string]any{
				"target": target, "command": command,
				"timeout_ms": timeoutMS(&g), "allow_public": g.AllowPublic,
				"jsonl": g.JSONL, "detach": g.Detach, "sensitive": g.Sensitive,
			}
			return call(cmd.Context(), wire.JobRun, params, &g, true)
		},
	}
	run.Flags().StringVar(&scriptFile, "script-file", "", "local script file; body is not parsed by PowerShell")
	run.Flags().StringVar(&runShell, "shell", "", "bash|sh|powershell|pwsh|raw (default: shebang or raw)")
	addGlobal(run, &g)

	cp := &cobra.Command{
		Use:   "cp [flags] <src> <dst>",
		Short: "Copy a single file over SFTP",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 2 {
				return exitErr(wire.E("INVALID_ARGUMENT", "cp requires <src> <dst>"), &g)
			}
			params := map[string]any{"src": args[0], "dst": args[1], "timeout_ms": timeoutMS(&g), "allow_public": g.AllowPublic}
			return call(cmd.Context(), wire.FileCopy, params, &g, false)
		},
	}
	addGlobal(cp, &g)

	serial := &cobra.Command{Use: "serial", Short: "Serial port commands"}
	var wait string
	var monAfter int64
	sexec := &cobra.Command{
		Use:  "exec [flags] <target> <payload>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			params := map[string]any{"target": args[0], "payload": args[1], "wait": wait, "timeout_ms": timeoutMS(&g), "allow_public": g.AllowPublic}
			return call(cmd.Context(), wire.SerialExec, params, &g, false)
		},
	}
	sexec.Flags().StringVar(&wait, "wait", "", "literal or re: pattern")
	addGlobal(sexec, &g)
	slist := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.SerialList, map[string]any{}, &g, false)
	}}
	addGlobal(slist, &g)
	smon := &cobra.Command{Use: "monitor [flags] <target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		g.JSONL = true
		return call(cmd.Context(), wire.SerialMonitor, map[string]any{"target": args[0], "after_cursor": monAfter, "allow_public": g.AllowPublic}, &g, true)
	}}
	smon.Flags().Int64Var(&monAfter, "after", 0, "after cursor")
	addGlobal(smon, &g)
	serial.AddCommand(sexec, slist, smon)

	var recipe, artifact, idem string
	deploy := &cobra.Command{
		Use:   "deploy [flags] <target>",
		Short: "Apply a recipe to an SSH host",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if recipe == "" || artifact == "" {
				return exitErr(wire.E("INVALID_ARGUMENT", "--recipe and --artifact are required"), &g)
			}
			params := map[string]any{"target": args[0], "recipe": recipe, "artifact": artifact, "idempotency_key": idem, "timeout_ms": timeoutMS(&g), "allow_public": g.AllowPublic}
			return call(cmd.Context(), wire.DeployStart, params, &g, false)
		},
	}
	deploy.Flags().StringVar(&recipe, "recipe", "", "recipe name")
	deploy.Flags().StringVar(&artifact, "artifact", "", "local artifact file")
	deploy.Flags().StringVar(&idem, "idempotency-key", "", "idempotency key")
	addGlobal(deploy, &g)

	jobCmd := &cobra.Command{Use: "job"}
	jwait := &cobra.Command{Use: "wait [flags] <job-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.JobWait, map[string]any{"job_id": args[0]}, &g, false)
	}}
	addGlobal(jwait, &g)
	var after int64
	jfollow := &cobra.Command{Use: "follow [flags] <job-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !g.JSON {
			g.JSONL = true
		}
		return call(cmd.Context(), wire.JobFollow, map[string]any{"job_id": args[0], "after_cursor": after}, &g, true)
	}}
	jfollow.Flags().Int64Var(&after, "after", 0, "replay after cursor")
	addGlobal(jfollow, &g)
	jcancel := &cobra.Command{Use: "cancel [flags] <job-id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.JobCancel, map[string]any{"job_id": args[0]}, &g, false)
	}}
	addGlobal(jcancel, &g)
	jlist := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.JobList, map[string]any{}, &g, false)
	}}
	addGlobal(jlist, &g)
	jobCmd.AddCommand(jwait, jfollow, jcancel, jlist)

	target := &cobra.Command{Use: "target"}
	tlist := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.TargetList, map[string]any{}, &g, false)
	}}
	addGlobal(tlist, &g)
	target.AddCommand(tlist)

	sess := &cobra.Command{Use: "session"}
	var sname string
	var sessAfter int64
	var waitDur string
	var readB64 bool
	var noSentinel bool
	var writeB64 string
	sopen := &cobra.Command{Use: "open [flags] <target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.SessionOpen, map[string]any{"target": args[0], "name": sname, "allow_public": g.AllowPublic}, &g, false)
	}}
	sopen.Flags().StringVar(&sname, "name", "", "session name")
	addGlobal(sopen, &g)
	sex := &cobra.Command{Use: "exec [flags] <session> -- <command...>", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return exitErr(wire.E("INVALID_ARGUMENT", "command required"), &g)
		}
		return call(cmd.Context(), wire.SessionExec, map[string]any{"session_id": args[0], "command": strings.Join(args[1:], " "), "timeout_ms": timeoutMS(&g), "no_sentinel": noSentinel}, &g, false)
	}}
	sex.Flags().BoolVar(&noSentinel, "no-sentinel", false, "do not wrap with exit sentinel")
	addGlobal(sex, &g)
	sread := &cobra.Command{Use: "read [flags] <session>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		waitMS := int64(0)
		if waitDur != "" {
			d, err := time.ParseDuration(waitDur)
			if err != nil {
				return exitErr(wire.E("INVALID_ARGUMENT", "invalid --wait"), &g)
			}
			waitMS = d.Milliseconds()
		}
		return call(cmd.Context(), wire.SessionRead, map[string]any{
			"session_id": args[0], "after_cursor": sessAfter, "wait_ms": waitMS, "bytes_b64": readB64, "jsonl": g.JSONL,
		}, &g, false)
	}}
	sread.Flags().Int64Var(&sessAfter, "after", 0, "after cursor")
	sread.Flags().StringVar(&waitDur, "wait", "", "wait duration such as 2s")
	sread.Flags().BoolVar(&readB64, "bytes-b64", false, "return data as base64")
	addGlobal(sread, &g)
	swrite := &cobra.Command{Use: "write [flags] <session> -- <data>", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]any{"session_id": args[0]}
		if writeB64 != "" {
			params["data_base64"] = writeB64
		} else if len(args) > 1 {
			params["data"] = strings.Join(args[1:], " ")
		} else {
			params["data"] = ""
		}
		return call(cmd.Context(), wire.SessionWrite, params, &g, false)
	}}
	swrite.Flags().StringVar(&writeB64, "bytes-b64", "", "base64 payload")
	addGlobal(swrite, &g)
	var cols, rows int
	sresize := &cobra.Command{Use: "resize [flags] <session>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.SessionResize, map[string]any{"session_id": args[0], "cols": cols, "rows": rows}, &g, false)
	}}
	sresize.Flags().IntVar(&cols, "cols", 80, "columns")
	sresize.Flags().IntVar(&rows, "rows", 24, "rows")
	addGlobal(sresize, &g)
	sattach := &cobra.Command{Use: "attach [flags] <session>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if g.JSONL {
			return call(cmd.Context(), wire.SessionAttach, map[string]any{"session_id": args[0]}, &g, true)
		}
		if err := attachSession(cmd.Context(), args[0], &g, true); err != nil {
			return exitErr(err, &g)
		}
		return nil
	}}
	addGlobal(sattach, &g)
	sdetach := &cobra.Command{Use: "detach [flags] <session>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.SessionDetach, map[string]any{"session_id": args[0]}, &g, false)
	}}
	addGlobal(sdetach, &g)
	sclose := &cobra.Command{Use: "close [flags] <session>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.SessionClose, map[string]any{"session_id": args[0]}, &g, false)
	}}
	addGlobal(sclose, &g)
	slogs := &cobra.Command{Use: "logs [flags] <session>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		params := map[string]any{"session_id": args[0], "after_cursor": int64(0), "jsonl": g.JSONL}
		if g.JSONL {
			r, err := invoke(cmd.Context(), wire.SessionRead, params, &g)
			if err != nil {
				return exitErr(err, &g)
			}
			if evs, ok := r["events"].([]any); ok {
				enc := json.NewEncoder(os.Stdout)
				enc.SetEscapeHTML(false)
				for _, ev := range evs {
					_ = enc.Encode(ev)
				}
			}
			os.Exit(exitFrom(true, r))
			return nil
		}
		return call(cmd.Context(), wire.SessionRead, params, &g, false)
	}}
	addGlobal(slogs, &g)
	sess.AddCommand(sopen, sex, sread, swrite, sresize, sattach, sdetach, sclose, slogs)

	shell := &cobra.Command{Use: "shell [flags] <target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !isTTY() {
			return exitErr(wire.E("INVALID_ARGUMENT", "hub shell requires a TTY; use session open/exec"), &g)
		}
		r, err := invoke(cmd.Context(), wire.SessionOpen, map[string]any{"target": args[0], "name": sname, "allow_public": g.AllowPublic}, &g)
		if err != nil {
			return exitErr(err, &g)
		}
		ok, _ := r["ok"].(bool)
		id, _ := r["operation_id"].(string)
		if !ok || id == "" {
			return finish(r, &g)
		}
		if err := attachSession(cmd.Context(), id, &g, true); err != nil {
			return exitErr(err, &g)
		}
		return nil
	}}
	shell.Flags().StringVar(&sname, "name", "", "session name")
	addGlobal(shell, &g)

	conn := &cobra.Command{Use: "connection"}
	clist := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.ConnectionList, map[string]any{}, &g, false)
	}}
	addGlobal(clist, &g)
	cstat := &cobra.Command{Use: "status [flags] <id-or-target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.ConnectionStatus, map[string]any{"id": args[0]}, &g, false)
	}}
	addGlobal(cstat, &g)
	cclose := &cobra.Command{Use: "close [flags] <id-or-target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.ConnectionClose, map[string]any{"id": args[0]}, &g, false)
	}}
	addGlobal(cclose, &g)
	conn.AddCommand(clist, cstat, cclose)

	secret := &cobra.Command{Use: "secret"}
	sset := &cobra.Command{Use: "set [flags] <target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		pw, err := readSecret()
		if err != nil {
			return exitErr(err, &g)
		}
		return call(cmd.Context(), wire.SecretSet, map[string]any{"target": args[0], "secret": pw}, &g, false)
	}}
	addGlobal(sset, &g)
	sdel := &cobra.Command{Use: "delete [flags] <target>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return call(cmd.Context(), wire.SecretDelete, map[string]any{"target": args[0]}, &g, false)
	}}
	addGlobal(sdel, &g)
	secret.AddCommand(sset, sdel)

	root.AddCommand(run, cp, serial, deploy, jobCmd, target, sess, shell, conn, secret)
	root.AddCommand(&cobra.Command{Use: "jobs", Hidden: true, RunE: jlist.RunE})
	root.AddCommand(&cobra.Command{Use: "logs", Args: cobra.ExactArgs(1), Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		g.JSONL = true
		return call(cmd.Context(), wire.JobFollow, map[string]any{"job_id": args[0], "after_cursor": 0}, &g, true)
	}})
	return root
}

func RejectLeadingFlags(args []string) error {
	return rejectFlagsBeforeVerb(args)
}

func rejectPasswordArgv(args []string) error {
	for _, a := range args {
		if a == "--" {
			return nil
		}
		if a == "--password" || strings.HasPrefix(a, "--password=") {
			return wire.E("INVALID_ARGUMENT", "passwords must not appear on argv; use hub secret set")
		}
	}
	return nil
}

func rejectFlagsBeforeVerb(args []string) error {
	for _, a := range args {
		if a == "--" || a == "--help" || a == "-h" || a == "help" || a == "--version" {
			return nil
		}
		if strings.HasPrefix(a, "-") {
			return wire.E("INVALID_ARGUMENT", "global flags must follow the verb")
		}
		return nil
	}
	return nil
}

func isTTY() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

func exitErr(err error, g *globals) error {
	printErr(err, g)
	os.Exit(wire.ExitCode(codeOf(err)))
	return err
}

func codeOf(err error) string {
	if e, ok := err.(*wire.Error); ok {
		return e.Code
	}
	return "INVALID_ARGUMENT"
}

func printErr(err error, g *globals) {
	e, ok := err.(*wire.Error)
	if !ok {
		e = wire.E("INVALID_ARGUMENT", err.Error())
	}
	if g.JSON || g.JSONL {
		enc := json.NewEncoder(os.Stdout)
		_ = enc.Encode(wire.Fail(wire.NewRequestID(), e))
		return
	}
	fmt.Fprintln(os.Stderr, e.Error())
}

func call(ctx context.Context, method string, params map[string]any, g *globals, stream bool) error {
	if timeoutMS(g) < 0 {
		return exitErr(wire.E("INVALID_ARGUMENT", "invalid --timeout"), g)
	}
	conn, err := ensureDaemon(ctx)
	if err != nil {
		printErr(err, g)
		os.Exit(3)
	}
	defer conn.Close()
	id := wire.NewRequestID()
	fr := &wire.Frame{V: wire.ProtocolVersion, Kind: wire.KindRequest, ID: id, Method: method, Params: wire.MustJSON(params)}
	if err := wire.WriteFrame(conn, fr); err != nil {
		printErr(wire.E("IPC_PROTOCOL_ERROR", err.Error()), g)
		os.Exit(3)
	}
	max := 16 * 1024 * 1024
	var final wire.Result
	for {
		in, err := wire.ReadFrame(conn, max)
		if err != nil {
			if err == io.EOF {
				break
			}
			if e, ok := err.(*wire.Error); ok {
				printErr(e, g)
				os.Exit(wire.ExitCode(e.Code))
			}
			printErr(wire.E("IPC_PROTOCOL_ERROR", err.Error()), g)
			os.Exit(3)
		}
		switch in.Kind {
		case wire.KindEvent:
			if g.JSONL {
				_, _ = os.Stdout.Write(append(in.Event, '\n'))
			} else if !g.JSON && !g.Quiet {
				var ev wire.Event
				_ = json.Unmarshal(in.Event, &ev)
				if ev.Type == "stdout" {
					fmt.Fprint(os.Stdout, ev.Data)
				} else if ev.Type == "stderr" {
					fmt.Fprint(os.Stderr, ev.Data)
				}
			}
		case wire.KindResponse:
			var payload wire.Result
			_ = json.Unmarshal(in.Payload, &payload)
			if st, _ := payload["stream"].(bool); st {
				continue
			}
			final = payload
			return finish(final, g)
		}
	}
	if final == nil {
		printErr(wire.E("IPC_PROTOCOL_ERROR", "no response"), g)
		os.Exit(3)
	}
	return finish(final, g)
}

func finish(r wire.Result, g *globals) error {
	ok, _ := r["ok"].(bool)
	status, _ := r["status"].(string)
	if g.JSONL {
		os.Exit(exitFrom(ok, r))
		return nil
	}
	if g.JSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(r)
		os.Exit(exitFrom(ok, r))
		return nil
	}
	if !ok {
		if msg, _ := r["message"].(string); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
		} else if c, _ := r["error_code"].(string); c != "" {
			fmt.Fprintln(os.Stderr, c)
		}
	} else if !g.Quiet {
		if s, _ := r["stdout"].(string); s != "" {
			fmt.Fprint(os.Stdout, s)
		} else if s, _ := r["data"].(string); s != "" {
			fmt.Fprint(os.Stdout, s)
		}
	}
	_ = status
	os.Exit(exitFrom(ok, r))
	return nil
}

func exitFrom(ok bool, r wire.Result) int {
	if ok {
		return 0
	}
	c, _ := r["error_code"].(string)
	if c == "" {
		return 1
	}
	return wire.ExitCode(c)
}

func ensureDaemon(ctx context.Context) (io.ReadWriteCloser, error) {
	sid, err := platform.CurrentSID()
	if err != nil {
		return nil, err
	}
	if high, _ := platform.HighIntegrity(); high {
		c, err := platform.DialPipe(ctx, sid)
		if err != nil {
			return nil, wire.E("DAEMON_INSTANCE_CONFLICT", "elevated process will not start hubd")
		}
		return c, nil
	}
	dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	c, err := platform.DialPipe(dctx, sid)
	cancel()
	if err == nil {
		return c, nil
	}
	mu, err := platform.AcquireMutex(platform.BootstrapMutexName(sid), 10*time.Second)
	if err != nil {
		return nil, wire.E("DAEMON_INSTANCE_CONFLICT", err.Error())
	}
	defer mu.Close()
	dctx, cancel = context.WithTimeout(ctx, 500*time.Millisecond)
	c, err = platform.DialPipe(dctx, sid)
	cancel()
	if err == nil {
		return c, nil
	}
	hubd, err := platform.SameDirExecutable(os.Args[0], "hubd")
	if err != nil {
		exe, _ := os.Executable()
		hubd = filepath.Join(filepath.Dir(exe), "hubd.exe")
		if _, st := os.Stat(hubd); st != nil {
			return nil, wire.E("DAEMON_INSTANCE_CONFLICT", "hubd.exe not found next to hub.exe")
		}
	}
	if err := platform.StartDetached(hubd); err != nil {
		return nil, wire.Ef("DAEMON_INSTANCE_CONFLICT", "start hubd: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		dctx, cancel = context.WithTimeout(ctx, 400*time.Millisecond)
		c, err = platform.DialPipe(dctx, sid)
		cancel()
		if err == nil {
			return c, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, wire.E("DAEMON_INSTANCE_CONFLICT", "hubd did not open the pipe in time")
}
