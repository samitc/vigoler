package vigoler

import (
	"context"
	"fmt"
)

var ffmpegWrapper externalApp = externalApp{appLocation: "ffmpeg"}

func runFFmpegTestLiveVideo(port, durationInSec int) (WaitAble, string,  error) {
	addr := fmt.Sprint("udp://127.0.0.1:", port)
	wa, err := ffmpegWrapper.runCommandWait(context.Background(), "-re", "-f", "lavfi", "-i",
		fmt.Sprint("nullsrc=s=256x256:d=", durationInSec), "-f", "mpegts", addr)
	if err != nil {
		return nil, "",  err
	}
	return wa, addr,  nil
}
