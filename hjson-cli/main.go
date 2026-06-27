package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"runtime/debug"
	"strings"

	"github.com/hjson/hjson-go/v4"
)

// Can be set when building for example like this:
// go build -ldflags "-X main.Version=v3.0"
var Version string

func fixJSON(data []byte) []byte {
	data = bytes.Replace(data, []byte("\\u003c"), []byte("<"), -1)
	data = bytes.Replace(data, []byte("\\u003e"), []byte(">"), -1)
	data = bytes.Replace(data, []byte("\\u0026"), []byte("&"), -1)
	data = bytes.Replace(data, []byte("\\u0008"), []byte("\\b"), -1)
	data = bytes.Replace(data, []byte("\\u000c"), []byte("\\f"), -1)
	return data
}

func main() {

	flag.Usage = func() {
		fmt.Println("usage: hjson-cli [OPTIONS] [INPUT]")
		fmt.Println("hjson can be used to convert JSON from/to Hjson.")
		fmt.Println("")
		fmt.Println("hjson will read the given JSON/Hjson input file or read from stdin.")
		fmt.Println("")
		fmt.Println("Options:")
		flag.PrintDefaults()
	}

	var help = flag.Bool("h", false, "Show this screen.")
	var showJSON = flag.Bool("j", false, "Output as formatted JSON.")
	var showCompact = flag.Bool("c", false, "Output as JSON.")

	var indentBy = flag.String("indentBy", "    ", "The indent string.")
	var bracesSameLine = flag.Bool("bracesSameLine", false, "Print braces on the same line.")
	var omitRootBraces = flag.Bool("omitRootBraces", false, "Omit braces at the root.")
	var quoteAlways = flag.Bool("quoteAlways", false, "Always quote string values.")
	var showVersion = flag.Bool("v", false, "Show version.")
	var preserveKeyOrder = flag.Bool("preserveKeyOrder", false, "Preserve key order in objects/maps.")
	var preserveComments = flag.Bool("preserveComments", false, "Preserve comments in Hjson output (and key order in any output).")
	var allowKeysWithoutValues = flag.Bool("allowKeysWithoutValues", true, "Allow object keys that have no value, recorded with a null value (enabled by default; use -allowKeysWithoutValues=false to disable).")

	flag.Parse()
	if *help || flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}

	if *showVersion {
		if Version != "" {
			fmt.Println(Version)
		} else if bi, ok := debug.ReadBuildInfo(); ok {
			fmt.Println(bi.Main.Version)
		} else {
			fmt.Println("Unknown version")
		}
		os.Exit(0)
	}

	convert := func(data []byte) ([]byte, error) {
		var value interface{}
		var err error

		decOpt := hjson.DefaultDecoderOptions()
		decOpt.AllowKeysWithoutValues = *allowKeysWithoutValues
		if *preserveKeyOrder || *preserveComments {
			decOpt.WhitespaceAsComments = false
			var node *hjson.Node
			err = hjson.UnmarshalWithOptions(data, &node, decOpt)
			value = node
		} else {
			err = hjson.UnmarshalWithOptions(data, &value, decOpt)
		}
		if err != nil {
			return nil, err
		}

		var out []byte
		if *showCompact {
			if out, err = json.Marshal(value); err != nil {
				return nil, err
			}
			out = fixJSON(out)
		} else if *showJSON {
			if out, err = json.MarshalIndent(value, "", *indentBy); err != nil {
				return nil, err
			}
			out = fixJSON(out)
		} else {
			opt := hjson.DefaultOptions()
			opt.IndentBy = *indentBy
			opt.BracesSameLine = *bracesSameLine
			opt.EmitRootBraces = !*omitRootBraces
			opt.QuoteAlways = *quoteAlways
			opt.Comments = *preserveComments
			if out, err = hjson.MarshalWithOptions(value, opt); err != nil {
				return nil, err
			}
		}
		return out, nil
	}

	// With no input file and an interactive terminal on stdin, run an
	// interactive REPL where each Ctrl+C parses the buffered input.
	if flag.NArg() == 0 {
		if fi, statErr := os.Stdin.Stat(); statErr == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			runREPL(convert)
			return
		}
	}

	var data []byte
	var err error
	if flag.NArg() == 1 {
		data, err = ioutil.ReadFile(flag.Arg(0))
	} else {
		data, err = ioutil.ReadAll(os.Stdin)
	}
	if err != nil {
		panic(err)
	}

	out, err := convert(data)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(out))
}

// runREPL drives an interactive Hjson session. Each time the user presses
// Ctrl+C the buffered input is parsed and the result is printed, then the REPL
// waits for the next input. The user can quit by entering "q" or "exit" and
// pressing Ctrl+C, or by sending EOF (Ctrl+Z on Windows, Ctrl+D on Unix). The
// platform-specific input handling lives in replReadLoop().
func runREPL(convert func([]byte) ([]byte, error)) {
	const prompt = "hjson> "

	defer restoreConsole()

	fmt.Fprintln(os.Stderr, "Interactive Hjson REPL.")
	fmt.Fprintln(os.Stderr, "  - Type or paste your Hjson (multiple lines allowed).")
	fmt.Fprintln(os.Stderr, "  - Press Ctrl+C to convert what you have typed so far.")
	fmt.Fprintln(os.Stderr, "  - To quit: type 'q' (or 'exit') and press Ctrl+C.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprint(os.Stderr, prompt)

	submit := make(chan string)
	done := make(chan struct{})
	go replReadLoop(submit, done)

	for {
		select {
		case content := <-submit:
			content = strings.TrimSpace(content)
			if content == "q" || content == "exit" {
				fmt.Fprintln(os.Stderr, "\nBye.")
				return
			}
			fmt.Fprintln(os.Stderr)
			evalAndPrint(content, convert)
			fmt.Fprint(os.Stderr, prompt)
		case <-done:
			fmt.Fprintln(os.Stderr)
			return
		}
	}
}

// evalAndPrint parses content as Hjson and prints the converted result to
// stdout, or prints any error to stderr. Empty input is ignored.
func evalAndPrint(content string, convert func([]byte) ([]byte, error)) {
	if content == "" {
		return
	}
	if out, err := convert([]byte(content)); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
	} else {
		fmt.Println(string(out))
	}
}
