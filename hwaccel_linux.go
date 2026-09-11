//go:build linux && (amd64 || arm64) && !go264_nohwaccel

package go264

import (
	"github.com/oops1/go.264/internal/hwaccel"
	"github.com/oops1/go.264/internal/hwaccel/nvenc"
	"github.com/oops1/go.264/internal/hwaccel/vaapi"
)

func init() {
	hwaccel.Register(nvenc.Backend())
	hwaccel.Register(vaapi.Backend())
}
