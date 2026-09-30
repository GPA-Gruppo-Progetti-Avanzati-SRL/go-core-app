#!/usr/bin/env bash
# migrate-split.sh — adegua il codice allo split di go-core-app in subpackage (2026-09-30).
#
# I simboli non hanno cambiato nome, solo package: `core.Properties` → `properties.Properties`,
# `core.ConcurrentN` → `utils.ConcurrentN`, ... (tabella sotto). Unica eccezione, nella radice:
# `core.ApplicationError` → `core.Error`. Errori, validazione, FormatBytes e le costanti delle date
# restano in `core`.
# Lo script riscrive i qualificatori nei .go sotto <dir>, aggiunge ESPLICITAMENTE gli import dei
# subpackage usati (goimports da solo potrebbe scegliere un altro package omonimo: `utils`, `props`,
# `validate` esistono anche fuori da go-core) e poi passa goimports, che toglie l'import di core
# rimasto senza usi.
#
# Uso:   go-core-app/scripts/migrate-split.sh <dir>        (es. la root dell'app)
# Poi:   go build ./... — un nome locale che coincide con un package nuovo (una variabile `props`,
#        un parametro `utils`) è un errore di compilazione da sistemare a mano: lo script non lo nasconde.
#
# Riconosce l'alias con cui il file importa go-core-app (`core`, `coreapp`, ...). Un dot-import
# fa fallire lo script: lì non c'è un qualificatore da riscrivere.

set -euo pipefail

DIR="${1:?uso: migrate-split.sh <dir>}"
MOD="github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"

# goimports compilato con la toolchain corrente: un binario più vecchio non sa leggere i metodi
# generici di Go 1.27 (go-core-mongo, go-core-sql) e fallisce su quei file.
goimports() { go run golang.org/x/tools/cmd/goimports@latest "$@"; }

# package → simboli spostati lì
read -r -d '' TABLE <<'EOF' || true
properties Properties BindProps Inherit IsZeroStruct PropTag DefaultTag ValidateTag
observability NewServerMetrics MetricsConfig MetricsSettings NewTracer Tracer MetricLogHook SlogHandler ProfilingHandler HealthHandler
httpx GenerateHttpClientWithInstrumentation AddEndpointNameMetrics DefaultHttpClientTimeout ServeOnLifecycle WaitContext
cli Execute Exec ITaskRunner Task TaskConfig FlagDefinition
utils Encrypt Decrypt StringToDate StringToDatePtr StringToDateTime StringToDateTimePtr DateToString DateToStringPtr DatePtrToString DatePtrToStringPtr DateTimeToString StringPtrToString NowTime NowString GetTimestamp GetMidnight ErrDateParse ConcurrentTwo ConcurrentN ErrConcurrentPanic TaggedFields TaggedField GetHostname UnknownHostname
EOF

export MOD
changed=0
while IFS= read -r -d '' f; do
  # alias dell'import della radice: `alias "MOD"` oppure `"MOD"` (→ core)
  alias=$(perl -ne 'if (/^\s*(?:import\s+)?([\w.]+)?\s*"\Q$ENV{MOD}\E"\s*$/) { print defined $1 ? $1 : "core"; exit }' "$f")
  [ -z "$alias" ] && continue
  if [ "$alias" = "." ]; then echo "✖ $f: dot-import di go-core-app, da migrare a mano" >&2; exit 1; fi
  [ "$alias" = "_" ] && continue
  export ALIAS="$alias"

  # l'unico rinomino dentro la radice
  perl -pi -e 's/\b\Q$ENV{ALIAS}\E\.ApplicationError\b/$ENV{ALIAS}.Error/g' "$f"

  needed=()
  while read -r pkg syms; do
    [ -z "$pkg" ] && continue
    export PKG="$pkg" RE="$(echo "$syms" | tr ' ' '|')"
    perl -ne '$f=1 if /\b\Q$ENV{ALIAS}\E\.(?:$ENV{RE})\b/; END { exit($f ? 0 : 1) }' "$f" || continue
    perl -pi -e 's/\b\Q$ENV{ALIAS}\E\.($ENV{RE})\b/$ENV{PKG}.$1/g' "$f"
    needed+=("$pkg")
  done <<< "$TABLE"
  [ ${#needed[@]} -eq 0 ] && continue

  # import espliciti, subito dopo quello della radice; `import core "MOD"` su una riga sola
  # diventa un blocco
  for pkg in "${needed[@]}"; do
    grep -q "\"$MOD/$pkg\"" "$f" && continue
    export PKG="$pkg"
    perl -pi -e 'if (!$done && /"\Q$ENV{MOD}\E"\s*$/) { my $imp = "\"$ENV{MOD}/$ENV{PKG}\""; if (s/^import\s+(.*)$/import (\n\t$1\n\t$imp\n)/) {} else { $_ .= "\t$imp\n" } $done = 1 }' "$f"
  done
  goimports -w "$f"
  echo "✔ $f: ${needed[*]}"
  changed=$((changed+1))
done < <(find "$DIR" -name '*.go' -not -path '*/vendor/*' -print0)

echo "file aggiornati: $changed"
