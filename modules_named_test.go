package core

import (
	"testing"

	"go.uber.org/fx"
)

// Tipi di prova per lo smoke test dei provider nominati.
type namedClient struct{ id string }

type iNamedClient interface{ ID() string }

func (c *namedClient) ID() string { return c.id }

func newNamedClient(id string) *namedClient { return &namedClient{id: id} }

// resetLists azzera lo stato package-level tra i sotto-test.
func resetLists() {
	provideslist = nil
	invokelist = nil
	supply = nil
	populatelist = nil
}

func TestProvideNamed(t *testing.T) {
	t.Run("named result resolved by name", func(t *testing.T) {
		resetLists()

		Provide(func() string { return "mngr-id" }) // consumato da newNamedClient
		ProvideNamed(newNamedClient, "mngr")

		var got *namedClient
		app := fx.New(
			provides(),
			fx.Invoke(fx.Annotate(func(c *namedClient) { got = c }, fx.ParamTags(`name:"mngr"`))),
		)
		if err := app.Err(); err != nil {
			t.Fatalf("fx.New error: %v", err)
		}
		if got == nil || got.id != "mngr-id" {
			t.Fatalf("expected named client resolved, got %+v", got)
		}
	})

	t.Run("as + named combined", func(t *testing.T) {
		resetLists()

		Provide(func() string { return "primary-id" })
		ProvideAsNamed[iNamedClient](newNamedClient, "primary")

		var got iNamedClient
		app := fx.New(
			provides(),
			fx.Invoke(fx.Annotate(func(c iNamedClient) { got = c }, fx.ParamTags(`name:"primary"`))),
		)
		if err := app.Err(); err != nil {
			t.Fatalf("fx.New error: %v", err)
		}
		if got == nil || got.ID() != "primary-id" {
			t.Fatalf("expected named interface resolved, got %+v", got)
		}
	})
}
