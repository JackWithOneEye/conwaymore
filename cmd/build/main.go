package main

import (
	"fmt"
	"log"
	"os/exec"

	"github.com/evanw/esbuild/pkg/api"
)

func main() {
	cmd := exec.Command("go", "env", "GOROOT")
	stdout, err := cmd.Output()
	if err != nil {
		log.Fatalf("get GOROOT command failed (%v)", err)
	}
	out := string(stdout)
	goroot := out[:len(out)-1]

	buildOpts := api.BuildOptions{
		EntryPointsAdvanced: []api.EntryPoint{
			{
				InputPath:  "cmd/web/frontend/index.js",
				OutputPath: "index",
			},
			{
				InputPath:  "cmd/web/frontend/vendor.js",
				OutputPath: "vendor",
			},
			{
				InputPath:  "cmd/web/frontend/worker/worker.js",
				OutputPath: "worker",
			},
			{
				InputPath:  fmt.Sprintf("%s/lib/wasm/wasm_exec.js", goroot),
				OutputPath: "wasm_exec",
			},
		},
		External: []string{"./wasm_exec.js"},
		Outdir:   "cmd/web/assets/js",
		Bundle:   true,
		Platform: api.PlatformBrowser,
		Loader: map[string]api.Loader{
			".wasm": api.LoaderBinary,
		},
		Format: api.FormatESModule,
		Target: api.ESNext,
		Write:  true,
	}
	result := api.Build(buildOpts)
	if len(result.Errors) != 0 {
		log.Fatalf("esbuild failed (%v)", result.Errors)
	}
}
