package main

import (
	"fmt"

	"github.com/notedit/gst"
)

const gstVideoPipeline = "videotestsrc ! video/x-raw,framerate=15/1 ! x264enc aud=false bframes=0 speed-preset=veryfast key-int-max=30 ! video/x-h264,stream-format=avc,profile=baseline ! h264parse ! appsink name=videosink "

type Sample struct {
	Data     []byte
	Video    bool
	Duration uint64
}

func pullVideoSample(element *gst.Element, out chan *Sample) {

	for {

		sle, err := element.PullSample()
		if err != nil {
			if element.IsEOS() == true {
				fmt.Println("eos")
				return
			} else {
				fmt.Println(err)
				continue
			}
		}

		sample := &Sample{
			Data:     sle.Data,
			Video:    true,
			Duration: sle.Duration,
		}

		out <- sample
	}
}

func main() {

	samples := make(chan *Sample, 10)

	err := gst.CheckPlugins([]string{"x264", "videoparsersbad"})

	fmt.Println("AAAA")

	if err != nil {
		panic(err)
	}

	vpipeline, err := gst.ParseLaunch(gstVideoPipeline)

	if err != nil {
		panic(err)
	}

	velement := vpipeline.GetByName("videosink")
	vpipeline.SetState(gst.StatePlaying)

	go pullVideoSample(velement, samples)

	for sample := range samples {
		fmt.Println("duration ", sample.Duration)
	}
}
