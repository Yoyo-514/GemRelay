package tests

import (
	"os"
	"testing"

	"gemrelay/tests/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Run(m)) }
