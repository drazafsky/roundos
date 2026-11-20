// roundui is the offline RUI v1 developer toolchain. No shell service, DRM
// device, IPC socket or installed application is touched by any command.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"roundos/internal/rui/compile"
	"roundos/internal/rui/preview"
	"roundos/sdk/rui/bundle"
)

const usage = `roundui (RUI v1 development tools)
Usage:
  roundui validate <layout.rui.yaml>
  roundui compile [-o layout.rui.bin] [-go ids.go] [-package main] <layout.rui.yaml>
  roundui inspect [-json] <layout.rui.yaml|layout.rui.bin>
  roundui preview [-o preview.png] [-screen screen.id] [-theme dark|light|contrast] [-values sample.json] <layout.rui.yaml|layout.rui.bin>

The preview uses the unmodified RoundOS 0.5.0 v7 circular renderer and
inert sample state; its form/detail/list template refinements await M5.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "roundui:", err)
		os.Exit(1)
	}
}
func run(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, usage)
		return nil
	}
	cmd := args[0]
	fs := flag.NewFlagSet("roundui "+cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var output, goOut, packageName, screen, theme, values string
	var jsonOutput bool
	switch cmd {
	case "validate":
	case "compile":
		fs.StringVar(&output, "o", "", "compiled RUI output path")
		fs.StringVar(&goOut, "go", "", "optional Go constants output path")
		fs.StringVar(&packageName, "package", "main", "generated Go package")
	case "inspect":
		fs.BoolVar(&jsonOutput, "json", false, "emit JSON inspection summary")
	case "preview":
		fs.StringVar(&output, "o", "rui-preview.png", "PNG output path")
		fs.StringVar(&screen, "screen", "", "screen ID, defaults to first")
		fs.StringVar(&theme, "theme", "dark", "RoundOS theme")
		fs.StringVar(&values, "values", "", "optional sample binding JSON")
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("%v\n%s", err, usage)
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("expected exactly one layout path\n%s", usage)
	}
	input := fs.Arg(0)
	src, err := os.ReadFile(input)
	if err != nil {
		return err
	}
	switch cmd {
	case "validate":
		_, f, err := compile.Compile(src)
		if err != nil {
			return fmt.Errorf("%s:%w", input, err)
		}
		fmt.Fprintf(stdout, "VALID %s: %s; %d screens, %d bindings, %d actions\n", input, f.Document.Application.ID, len(f.Document.Screens), len(f.Manifest.Bindings), len(f.Manifest.Actions))
		return nil
	case "compile":
		encoded, f, err := compile.Compile(src)
		if err != nil {
			return fmt.Errorf("%s:%w", input, err)
		}
		if output == "" {
			output = strings.TrimSuffix(input, filepath.Ext(input)) + ".bin"
		}
		if filepath.Clean(output) == filepath.Clean(input) {
			return fmt.Errorf("refusing to overwrite input layout")
		}
		if goOut != "" && (filepath.Clean(goOut) == filepath.Clean(input) || filepath.Clean(goOut) == filepath.Clean(output)) {
			return fmt.Errorf("output file paths overlap")
		}
		var generated []byte
		if goOut != "" {
			generated, err = compile.GoConstants(f, packageName)
			if err != nil {
				return err
			}
		}
		if err = writeAtomic(output, encoded, 0644); err != nil {
			return err
		}
		if goOut != "" {
			if err = writeAtomic(goOut, generated, 0644); err != nil {
				return err
			}
		}
		fmt.Fprintf(stdout, "COMPILED %s -> %s (%d bytes, %d bindings, %d actions)\n", input, output, len(encoded), len(f.Manifest.Bindings), len(f.Manifest.Actions))
		return nil
	case "inspect", "preview":
		var f *bundle.File
		if bytes.HasPrefix(src, []byte("RUI1")) {
			f, err = compile.Verify(src)
		} else {
			_, f, err = compile.Compile(src)
		}
		if err != nil {
			return fmt.Errorf("%s:%w", input, err)
		}
		if cmd == "inspect" {
			data := inspect(f)
			if jsonOutput {
				b, e := json.MarshalIndent(data, "", "  ")
				if e != nil {
					return e
				}
				fmt.Fprintf(stdout, "%s\n", b)
			} else {
				fmt.Fprintf(stdout, "RUI %s | application %s\n", f.Format, f.Document.Application.ID)
				fmt.Fprintf(stdout, "  source SHA-256: %s\n", f.SourceSHA256)
				for _, sc := range f.Document.Screens {
					count := 0
					for _, g := range sc.Groups {
						count += len(g.Widgets)
						if g.Title != nil {
							count++
						}
					}
					fmt.Fprintf(stdout, "  screen %s: %s, %d effective widgets, %d groups\n", sc.ID, sc.Layout, count, len(sc.Groups))
				}
				fmt.Fprintln(stdout, "  bindings:")
				for _, b := range data.Bindings {
					fmt.Fprintf(stdout, "    %s: %s\n", b.Name, b.Type)
				}
				fmt.Fprintln(stdout, "  actions:")
				for _, a := range f.Manifest.Actions {
					fmt.Fprintf(stdout, "    %s: %s\n", a.Name, a.ValueType)
				}
			}
			return nil
		}
		if theme != "dark" && theme != "light" && theme != "contrast" {
			return fmt.Errorf("invalid theme %q", theme)
		}
		var bindings []byte
		if values != "" {
			bindings, err = os.ReadFile(values)
			if err != nil {
				return err
			}
			if len(bindings) > preview.MaxValuesJSON {
				return fmt.Errorf("sample binding file too large")
			}
		}
		state, err := preview.NewState(f.Manifest, bindings)
		if err != nil {
			return err
		}
		view, err := preview.View(f, state)
		if err != nil {
			return err
		}
		frame, err := preview.Render(f, view, screen, theme)
		if err != nil {
			return err
		}
		buf := &bytes.Buffer{}
		if err = png.Encode(buf, frame); err != nil {
			return err
		}
		if filepath.Clean(output) == filepath.Clean(input) {
			return fmt.Errorf("refusing to overwrite input layout")
		}
		if err = writeAtomic(output, buf.Bytes(), 0644); err != nil {
			return err
		}
		n, _ := preview.FrameBytes(view)
		fmt.Fprintf(stdout, "PREVIEW %s (%dx%d, %d-byte v7 view IPC frame, %d sample bindings missing)\n", output, frame.Bounds().Dx(), frame.Bounds().Dy(), n, len(state.Missing()))
		if len(state.Missing()) > 0 {
			fmt.Fprintf(stdout, "Missing binding samples (using inert preview defaults): %s\n", strings.Join(state.Missing(), ", "))
		}
		fmt.Fprintln(stdout, "Note: v7 shell renders list/form/detail as the same row-list geometry; distinct template treatment and live binding dispatch are not implemented in M2.")
		return nil
	}
	return errors.New("unreachable command")
}

type bindingInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
type screenInfo struct {
	ID, Layout               string
	Groups, EffectiveWidgets int
}
type inspectInfo struct {
	BundleFormat  string        `json:"bundleFormat"`
	Schema        string        `json:"schema"`
	ApplicationID string        `json:"applicationID"`
	SourceSHA256  string        `json:"sourceSHA256"`
	Screens       []screenInfo  `json:"screens"`
	Bindings      []bindingInfo `json:"bindings"`
	Actions       any           `json:"actions"`
	SourceMap     any           `json:"sourceMap"`
}

func inspect(f *bundle.File) inspectInfo {
	d := inspectInfo{BundleFormat: f.Format, Schema: f.Document.Schema, ApplicationID: f.Document.Application.ID, SourceSHA256: f.SourceSHA256, Actions: f.Manifest.Actions, SourceMap: f.SourceMap, Screens: []screenInfo{}, Bindings: []bindingInfo{}}
	for _, sc := range f.Document.Screens {
		n := 0
		for _, g := range sc.Groups {
			n += len(g.Widgets)
			if g.Title != nil {
				n++
			}
		}
		d.Screens = append(d.Screens, screenInfo{sc.ID, sc.Layout, len(sc.Groups), n})
	}
	for k, v := range f.Manifest.Bindings {
		d.Bindings = append(d.Bindings, bindingInfo{k, string(v)})
	}
	for i := 1; i < len(d.Bindings); i++ {
		for j := i; j > 0 && d.Bindings[j].Name < d.Bindings[j-1].Name; j-- {
			d.Bindings[j], d.Bindings[j-1] = d.Bindings[j-1], d.Bindings[j]
		}
	}
	return d
}
func writeAtomic(path string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".roundui-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
