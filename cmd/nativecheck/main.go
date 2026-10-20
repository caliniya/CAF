// nativecheck loads and resolves exports only. It does not create a QQ session or log in.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/caliniya/CAF-Go/internal/lagrange"
	"github.com/caliniya/CAF-Go/internal/paths"
)

func main() {
	data := flag.String("data-dir", os.Getenv("CAF_DATA_DIR"), "data directory (absolute recommended with go run)")
	flag.Parse()
	directories, err := paths.Discover(*data)
	if err == nil {
		_, err = lagrange.NewLoader().Load(directories.Data)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "loaded", "message": "6 exports resolved; no QQ session or login started"})
}
