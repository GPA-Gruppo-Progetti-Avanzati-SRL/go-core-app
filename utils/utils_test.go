package utils

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
)

func TestConcurrentN_ConcorrenzaNonPositiva(t *testing.T) {
	for _, c := range []int{0, -3} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			out, err := ConcurrentN([]int{1, 2, 3}, c, func(i int) (int, *core.Error) { return i * 2, nil })
			if err != nil || len(out) != 3 || out[2] != 6 {
				t.Errorf("concurrency=%d: out=%v err=%v", c, out, err)
			}
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("concurrency=%d: ConcurrentN bloccata", c)
		}
	}
}

func TestConcurrentN_PanicDiventaErrore(t *testing.T) {
	_, err := ConcurrentN([]int{1, 2}, 2, func(i int) (int, *core.Error) {
		if i == 2 {
			panic("boom")
		}
		return i, nil
	})
	if err == nil || err.Code != ErrConcurrentPanic {
		t.Fatalf("atteso %s, got %v", ErrConcurrentPanic, err)
	}
	if _, _, err := ConcurrentTwo(
		func() (int, *core.Error) { return 1, nil },
		func() (int, *core.Error) { panic("boom") },
	); err == nil || err.Code != ErrConcurrentPanic {
		t.Fatalf("ConcurrentTwo: atteso %s, got %v", ErrConcurrentPanic, err)
	}
}

func TestGetHostname_MaiVuoto(t *testing.T) {
	if h := GetHostname(); h == "" || h != GetHostname() {
		t.Fatalf("GetHostname = %q", h)
	}
}

func TestTaggedFields(t *testing.T) {
	type filtro struct {
		interno string // non esportato e senza tag: prima faceva panicare go-core-mongo
		Nome    string `col:"name" op:"="`
		Eta     int    `col:"age" op:">" omitempty:""`
		Libero  string
	}
	got, err := TaggedFields(&filtro{interno: "x", Nome: "ada"}, "col", "op")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "name" || got[0].Op != "=" || got[0].Value != "ada" {
		t.Fatalf("TaggedFields = %+v", got)
	}

	type malTaggato struct {
		nome string `col:"name" op:"="`
	}
	if _, err := TaggedFields(malTaggato{nome: "x"}, "col", "op"); err == nil {
		t.Fatal("campo non esportato con i tag: atteso errore, non panic")
	}
	var nilPtr *filtro
	for _, in := range []any{nil, nilPtr, 3} {
		if _, err := TaggedFields(in, "col", "op"); err == nil {
			t.Fatalf("TaggedFields(%v): atteso errore", in)
		}
	}
}

func TestEncryptDecrypt_RoundTripViaHex(t *testing.T) {
	ct, err := Encrypt([]byte("segreto"), "app-id")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Decrypt(hex.EncodeToString(ct), "app-id")
	if err != nil || string(pt) != "segreto" {
		t.Fatalf("round-trip = %q, %v", pt, err)
	}
	if _, err := Decrypt(hex.EncodeToString(ct), "altra-app"); err == nil {
		t.Fatal("chiave diversa: atteso errore")
	}
}
