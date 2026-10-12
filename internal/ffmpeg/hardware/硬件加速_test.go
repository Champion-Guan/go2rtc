package hardware

import (
	"testing"

	pkgffmpeg "github.com/AlexxIT/go2rtc/pkg/ffmpeg"
	"github.com/stretchr/testify/require"
)

func TestMakeHardwareAutoH265(t *testing.T) {
	oldProbe := probeHardware
	oldCache := cache
	cache = map[string]string{}
	t.Cleanup(func() {
		probeHardware = oldProbe
		cache = oldCache
	})

	var called int
	probeHardware = func(bin, name string) string {
		called++
		require.Equal(t, "ffmpeg", bin)
		require.Equal(t, "h265", name)
		return EngineVideoToolbox
	}

	args := &pkgffmpeg.Args{
		Bin:    "ffmpeg",
		Input:  "-i input.mp4",
		Codecs: []string{"-c:v libx265 -g 50"},
	}
	defaults := map[string]string{
		"h265/videotoolbox": "-c:v hevc_videotoolbox -g 50",
	}

	MakeHardware(args, "", defaults)

	require.Equal(t, 1, called)
	require.Equal(t, "-hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld -i input.mp4", args.Input)
	require.Equal(t, []string{"-c:v hevc_videotoolbox -g 50"}, args.Codecs)
}

func TestMakeHardwareCachesAutoH265Probe(t *testing.T) {
	oldProbe := probeHardware
	oldCache := cache
	cache = map[string]string{}
	t.Cleanup(func() {
		probeHardware = oldProbe
		cache = oldCache
	})

	var called int
	probeHardware = func(bin, name string) string {
		called++
		return EngineVideoToolbox
	}

	defaults := map[string]string{
		"h265/videotoolbox": "-c:v hevc_videotoolbox -g 50",
	}

	first := &pkgffmpeg.Args{
		Bin:    "ffmpeg",
		Input:  "-i first.mp4",
		Codecs: []string{"-c:v libx265 -g 50"},
	}
	MakeHardware(first, "", defaults)

	second := &pkgffmpeg.Args{
		Bin:    "ffmpeg",
		Input:  "-i second.mp4",
		Codecs: []string{"-c:v libx265 -g 50"},
	}
	MakeHardware(second, "", defaults)

	require.Equal(t, 1, called)
	require.Equal(t, []string{"-c:v hevc_videotoolbox -g 50"}, first.Codecs)
	require.Equal(t, []string{"-c:v hevc_videotoolbox -g 50"}, second.Codecs)
}

func TestMakeHardwareVideoToolboxUsesHWFiltersWhenSupported(t *testing.T) {
	args := &pkgffmpeg.Args{
		Bin:     "ffmpeg",
		Input:   "-i input.mp4",
		Codecs:  []string{"-c:v libx264 -g 50"},
		Filters: []string{"scale=1920:1080"},
	}
	defaults := map[string]string{
		"h264/videotoolbox": "-c:v h264_videotoolbox -g 50",
	}

	MakeHardware(args, EngineVideoToolbox, defaults)

	require.Equal(t, "-hwaccel videotoolbox -hwaccel_output_format videotoolbox_vld -i input.mp4", args.Input)
	require.Equal(t, []string{"-c:v h264_videotoolbox -g 50"}, args.Codecs)
	require.Equal(t, []string{"scale_vt=1920:1080"}, args.Filters)
}

func TestMakeHardwareVideoToolboxFallsBackToNV12ForSoftwareFilters(t *testing.T) {
	args := &pkgffmpeg.Args{
		Bin:     "ffmpeg",
		Input:   "-i input.mp4",
		Codecs:  []string{"-c:v libx264 -g 50"},
		Filters: []string{"drawtext=fontsize=12:text=test"},
	}
	defaults := map[string]string{
		"h264/videotoolbox": "-c:v h264_videotoolbox -g 50",
	}

	MakeHardware(args, EngineVideoToolbox, defaults)

	require.Equal(t, "-hwaccel videotoolbox -hwaccel_output_format nv12 -i input.mp4", args.Input)
	require.Equal(t, []string{"-c:v h264_videotoolbox -g 50"}, args.Codecs)
	require.Equal(t, []string{"drawtext=fontsize=12:text=test"}, args.Filters)
}
