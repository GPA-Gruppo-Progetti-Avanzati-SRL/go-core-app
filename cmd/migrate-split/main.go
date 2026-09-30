// Command migrate-split adegua il codice di un'applicazione allo split di go-core-app in package per
// dominio (2026-09-30).
//
//	go run github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/cmd/migrate-split@latest <dir>...
//
// I simboli non hanno cambiato nome, solo package — `core.Properties` → `properties.Properties`,
// `core.ConcurrentN` → `utils.ConcurrentN`, ... — con un'eccezione dentro la radice:
// `core.ApplicationError` → `core.Error`. Il tool lavora sull'AST: riscrive solo i selettori il cui
// qualificatore è l'import di go-core-app (col suo alias, qualunque sia), quindi non tocca commenti,
// stringhe, campi omonimi (`s.core.Task`) né file generati. Aggiunge gli import dei package nuovi,
// toglie quello della radice se non serve più, e formatta il file.
//
// Se il nome del package nuovo è già usato nel file (una variabile `properties`, un parametro
// `utils`), l'import prende un alias (`coreproperties`, `coreutils`) invece di rompere il codice.
// Un dot-import di go-core-app non ha qualificatori da riscrivere: il file è elencato e lasciato
// com'è. Il comando esce con 1 se un file non è stato migrato.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Module è il path del modulo go-core-app.
const Module = "github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"

// Moved dice in quale package è finito ogni simbolo che ha lasciato la radice.
var Moved = map[string][]string{
	"properties": {"Properties", "BindProps", "Inherit", "IsZeroStruct", "PropTag", "DefaultTag", "ValidateTag"},
	"observability": {"NewServerMetrics", "MetricsConfig", "MetricsSettings", "NewTracer", "Tracer", "MetricLogHook",
		"SharedMetricLogHook", "SlogHandler", "ProfilingHandler", "HealthHandler",
		"DefaultMetricsHost", "DefaultMetricsPort", "DefaultMetricsReadHeaderTimeout"},
	"httpx": {"GenerateHttpClientWithInstrumentation", "AddEndpointNameMetrics", "DefaultHttpClientTimeout", "ServeOnLifecycle"},
	"cli":   {"Execute", "Exec", "ITaskRunner", "Task", "TaskConfig", "FlagDefinition"},
	"utils": {"Encrypt", "Decrypt", "StringToDate", "StringToDatePtr", "StringToDateTime", "StringToDateTimePtr",
		"DateToString", "DateToStringPtr", "DatePtrToString", "DatePtrToStringPtr", "DateTimeToString", "StringPtrToString",
		"NowTime", "NowString", "GetTimestamp", "GetMidnight", "ErrDateParse", "ConcurrentTwo", "ConcurrentN",
		"ErrConcurrentPanic", "TaggedFields", "TaggedField", "GetHostname", "UnknownHostname"},
}

// Renamed sono i simboli rinominati restando nella radice.
var Renamed = map[string]string{"ApplicationError": "Error"}

var symbolPkg = func() map[string]string {
	m := map[string]string{}
	for pkg, syms := range Moved {
		for _, s := range syms {
			m[s] = pkg
		}
	}
	return m
}()

// Result è l'esito della migrazione di un file.
type Result struct {
	Changed bool
	Pkgs    []string // package aggiunti agli import
	Skipped string   // non vuoto se il file va migrato a mano, col motivo
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: migrate-split <dir>...")
		os.Exit(2)
	}
	var changed, skipped int
	for _, root := range os.Args[1:] {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out, res, err := Migrate(path, src)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			switch {
			case res.Skipped != "":
				skipped++
				fmt.Printf("✖ %s: %s\n", path, res.Skipped)
			case res.Changed:
				changed++
				if err := os.WriteFile(path, out, 0o644); err != nil {
					return err
				}
				fmt.Printf("✔ %s %v\n", path, res.Pkgs)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	fmt.Printf("file aggiornati: %d, da migrare a mano: %d\n", changed, skipped)
	if skipped > 0 {
		os.Exit(1)
	}
}

type edit struct {
	start, end int
	text       string
}

// Migrate ritorna il sorgente migrato. Un file che non importa go-core-app, o generato, esce
// invariato.
func Migrate(filename string, src []byte) ([]byte, Result, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, Result{}, err
	}
	if ast.IsGenerated(f) {
		return src, Result{}, nil
	}
	var rootSpec *ast.ImportSpec
	for _, is := range f.Imports {
		if p, _ := strconv.Unquote(is.Path.Value); p == Module {
			rootSpec = is
		}
	}
	if rootSpec == nil {
		return src, Result{}, nil
	}
	alias := "core"
	if rootSpec.Name != nil {
		alias = rootSpec.Name.Name
	}
	switch alias {
	case "_":
		return src, Result{}, nil
	case ".":
		return src, Result{Skipped: "dot-import di go-core-app: non ci sono qualificatori da riscrivere"}, nil
	}

	// Nomi già dichiarati nel file: il package nuovo non può prenderne uno.
	declared := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Obj != nil {
			declared[id.Name] = true
		}
		return true
	})
	for _, is := range f.Imports {
		if is.Name != nil {
			declared[is.Name.Name] = true
		} else if p, _ := strconv.Unquote(is.Path.Value); p != Module {
			declared[filepath.Base(p)] = true
		}
	}

	offset := func(p token.Pos) int { return fset.Position(p).Offset }
	var edits []edit
	importName := map[string]string{} // package nuovo → nome con cui si importa
	remaining := 0
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != alias || x.Obj != nil {
			return true
		}
		if newName, ok := Renamed[sel.Sel.Name]; ok {
			edits = append(edits, edit{offset(sel.Sel.Pos()), offset(sel.Sel.End()), newName})
			remaining++
			return true
		}
		pkg, ok := symbolPkg[sel.Sel.Name]
		if !ok {
			remaining++
			return true
		}
		name, ok := importName[pkg]
		if !ok {
			name = pkg
			if declared[name] {
				name = "core" + pkg
			}
			importName[pkg] = name
		}
		edits = append(edits, edit{offset(x.Pos()), offset(x.End()), name})
		return true
	})
	if len(edits) == 0 {
		return src, Result{}, nil
	}

	pkgs := make([]string, 0, len(importName))
	for p := range importName {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	var lines []string
	for _, p := range pkgs {
		spec := strconv.Quote(Module + "/" + p)
		if importName[p] != p {
			spec = importName[p] + " " + spec
		}
		lines = append(lines, spec)
	}

	// La dichiarazione d'import che contiene la radice: se è un blocco si sostituisce la sola
	// riga della radice, se è una riga singola diventa un blocco.
	var decl *ast.GenDecl
	for _, d := range f.Decls {
		if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.IMPORT && slices.Contains(gd.Specs, ast.Spec(rootSpec)) {
			decl = gd
		}
	}
	if decl == nil {
		return nil, Result{}, errors.New("import di go-core-app fuori da una dichiarazione import")
	}
	// Fine della riga dell'import della radice, commento in coda compreso.
	eol := offset(rootSpec.End())
	if i := bytes.IndexByte(src[eol:], '\n'); i >= 0 {
		eol += i
	} else {
		eol = len(src)
	}
	rootLine := string(src[offset(rootSpec.Pos()):eol])
	switch {
	case len(lines) == 0:
		// Solo rinomini dentro la radice: gli import non cambiano.
	case !decl.Lparen.IsValid():
		all := lines
		if remaining > 0 {
			all = append([]string{rootLine}, lines...)
		}
		edits = append(edits, edit{offset(decl.Pos()), eol, "import (\n\t" + strings.Join(all, "\n\t") + "\n)"})
	case remaining > 0:
		// La radice resta, col suo commento: le righe nuove vanno dopo.
		edits = append(edits, edit{eol, eol, "\n\t" + strings.Join(lines, "\n\t")})
	default:
		// La radice non serve più: se ne va la riga intera, commento compreso.
		edits = append(edits, edit{offset(rootSpec.Pos()), eol, strings.Join(lines, "\n\t")})
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	out := slices.Clone(src)
	for _, e := range edits {
		out = append(out[:e.start:e.start], append([]byte(e.text), out[e.end:]...)...)
	}
	formatted, err := format.Source(out)
	if err != nil {
		return nil, Result{}, fmt.Errorf("sorgente migrato non valido: %w", err)
	}
	return formatted, Result{Changed: !bytes.Equal(formatted, src), Pkgs: pkgs}, nil
}
