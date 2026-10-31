package realtime

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePayload(t *testing.T) {
	id := uuid.New()
	e, err := ParsePayload(id.String() + ":item")
	require.NoError(t, err)
	assert.Equal(t, Event{ProjectID: id, Jenis: "item"}, e)
	for _, bad := range []string{"", "item", id.String(), id.String() + ":", "bukan-uuid:item"} {
		_, err := ParsePayload(bad)
		assert.Error(t, err, bad)
	}
}

func TestHubFiltersByProjectAndLimitsPerUser(t *testing.T) {
	h := NewHub(2)
	u1, u2 := uuid.New(), uuid.New()
	p1, p2 := uuid.New(), uuid.New()
	a, err := h.Subscribe(u1, []uuid.UUID{p1})
	require.NoError(t, err)
	b, err := h.Subscribe(u1, []uuid.UUID{p2})
	require.NoError(t, err)
	_, err = h.Subscribe(u1, []uuid.UUID{p1})
	assert.ErrorIs(t, err, ErrTooManyStreams)
	c, err := h.Subscribe(u2, []uuid.UUID{p1, p2})
	require.NoError(t, err)

	h.Publish(Event{ProjectID: p1, Jenis: "item"})
	assert.Equal(t, Event{ProjectID: p1, Jenis: "item"}, <-a.C())
	assert.Equal(t, p1, (<-c.C()).ProjectID)
	assert.Empty(t, b.C())

	// Hak diperbarui: b kini juga mengikuti p1.
	b.SetProjects([]uuid.UUID{p1})
	h.Publish(Event{ProjectID: p1, Jenis: "anggaran"})
	assert.Equal(t, "anggaran", (<-b.C()).Jenis)

	// Close idempoten dan membebaskan kuota pengguna.
	a.Close()
	a.Close()
	assert.Equal(t, 2, h.Count())
	_, err = h.Subscribe(u1, []uuid.UUID{p1})
	assert.NoError(t, err)

	// Antrean penuh tidak memblokir Publish.
	for range 200 {
		h.Publish(Event{ProjectID: p2, Jenis: "item"})
	}
	assert.Len(t, c.C(), cap(c.ch))

	select {
	case <-h.Done():
		t.Fatal("Done tertutup sebelum Shutdown")
	default:
	}
	h.Shutdown()
	h.Shutdown()
	<-h.Done()
}
