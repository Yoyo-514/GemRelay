package tests

import (
	"os"
	"testing"

	"gemini-web2api/tests/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.Run(m)) }
