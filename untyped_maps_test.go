package calendar

import (
	"testing"

	"github.com/localitas/localitas-go/maptest"
)

func TestNoUntypedMaps(t *testing.T) {
	maptest.NoUntypedMaps(t, ".", nil)
}
