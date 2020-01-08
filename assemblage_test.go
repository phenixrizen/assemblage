package assemblage

import (
	"testing"
	"time"

	"github.com/hashicorp/memberlist"
	cache "github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNewAssemblage(t *testing.T) {
	assert := assert.New(t)

	log := logrus.New()

	assem, _ := NewAssemblage(42280, "eth0", "test", "me", "now.", log)

	_assem := &Assemblage{
		port:       42280,
		service:    "test",
		instance:   "me",
		domain:     "now.",
		iface:      "eth0",
		memberlist: (*memberlist.Memberlist)(nil),
		cache:      (*cache.Cache)(nil),
		logger:     log,
		events:     (chan memberlist.NodeEvent)(nil),
	}

	assert.Equal(_assem, assem)
}

func TestAssemblageRun(t *testing.T) {
	assert := assert.New(t)

	log := logrus.New()

	assem, _ := NewAssemblage(42280, "eth0", "test", "me", "now.", log)

	err := assem.Run()
	assert.Equal(nil, err)

	time.Sleep(3 * time.Second)

	err = assem.Shutdown()
	assert.Equal(nil, err)
}
