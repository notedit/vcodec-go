package main

import (
	"fmt"

	"github.com/notedit/gst"
	"github.com/notedit/vcodec-go"
)

const gstVideoPipeline = "videotestsrc ! video/x-raw,framerate=10/1 ! x264enc aud=false bframes=0 speed-preset=veryfast key-int-max=30 ! video/x-h264,stream-format=byte-stream,profile=baseline ! h264parse ! appsink name=videosink "

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

	decoder, err := vcodec.NewVideoDecoder("h264")

	if err != nil {
		panic(err)
	}

	encoder, err := vcodec.NewVideoEncoder("libx264")

	if err != nil {
		panic(err)
	}

	fmt.Println(encoder)

	encoder.SetBitrate(500000)
	encoder.SetFramerate(10)
	encoder.SetGopsize(30)
	encoder.SetHeight(240)
	encoder.SetWidth(320)

	encoder.Setup()

	samples := make(chan *Sample, 10)

	err = gst.CheckPlugins([]string{"x264", "videoparsersbad"})

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
		frame, err := decoder.Decode(sample.Data)

		if err != nil {
			fmt.Println(err)
		}

		fmt.Println("decode frame", frame.Image.Bounds())

		_, pkt, err := encoder.Encode(frame)
		if err != nil {
			panic(err)
		}

		fmt.Println(len(pkt))
	}
}