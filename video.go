package vcodec

/*
#cgo LDFLAGS: -lavformat -lavutil -lavcodec -lswscale
#cgo CFLAGS: -Wno-deprecated
#include <libavformat/avformat.h>
#include <libavcodec/avcodec.h>
#include <libavutil/avutil.h>
#include <libswscale/swscale.h>
#include <libavutil/opt.h>
#include <string.h>

typedef struct {
	AVCodec *codec;
	AVCodecContext *codecCtx;
	AVFrame *frame;
	AVDictionary *options;
	int profile;
} FFCtx;

static inline int avcodec_profile_name_to_int(AVCodec *codec, const char *name) {
	const AVProfile *p;
	for (p = codec->profiles; p != NULL && p->profile != FF_PROFILE_UNKNOWN; p++)
		if (!strcasecmp(p->name, name))
			return p->profile;
	return FF_PROFILE_UNKNOWN;
}

void ffinit() {
	av_register_all();
}

int wrap_avcodec_decode_video2(AVCodecContext *ctx, AVFrame *frame, void *data, int size, int *got) {
	struct AVPacket pkt = {.data = data, .size = size};
	return avcodec_decode_video2(ctx, frame, got, &pkt);
}

*/
import "C"
import (
	"fmt"
	"image"
	"reflect"
	"runtime"
	"unsafe"
)

func init() {
	C.ffinit()
}

type ffctx struct {
	ff C.FFCtx
}

func newFFCtxByCodec(codec *C.AVCodec) (ff *ffctx, err error) {
	ff = &ffctx{}
	ff.ff.codec = codec
	ff.ff.codecCtx = C.avcodec_alloc_context3(codec)
	ff.ff.profile = C.FF_PROFILE_UNKNOWN
	runtime.SetFinalizer(ff, freeFFCtx)
	return
}

func freeFFCtx(self *ffctx) {
	ff := &self.ff
	if ff.frame != nil {
		C.av_frame_free(&ff.frame)
	}
	if ff.codecCtx != nil {
		C.avcodec_close(ff.codecCtx)
		C.av_free(unsafe.Pointer(ff.codecCtx))
		ff.codecCtx = nil
	}
	if ff.options != nil {
		C.av_dict_free(&ff.options)
	}
}

type VideoFrame struct {
	Image image.YCbCr
	frame *C.AVFrame
}

func (self *VideoFrame) Free() {
	self.Image = image.YCbCr{}
	C.av_frame_free(&self.frame)
}

func freeVideoFrame(self *VideoFrame) {
	self.Free()
}

type VideoDecoder struct {
	ff        *ffctx
	Extradata []byte
}

func (self *VideoDecoder) Setup() (err error) {
	ff := &self.ff.ff
	if len(self.Extradata) > 0 {
		ff.codecCtx.extradata = (*C.uint8_t)(unsafe.Pointer(&self.Extradata[0]))
		ff.codecCtx.extradata_size = C.int(len(self.Extradata))
	}
	if C.avcodec_open2(ff.codecCtx, ff.codec, nil) != 0 {
		err = fmt.Errorf("ffmpeg: decoder: avcodec_open2 failed")
		return
	}
	return
}

func fromCPtr(buf unsafe.Pointer, size int) (ret []uint8) {
	hdr := (*reflect.SliceHeader)((unsafe.Pointer(&ret)))
	hdr.Cap = size
	hdr.Len = size
	hdr.Data = uintptr(buf)
	return
}

func (self *VideoDecoder) Decode(pkt []byte) (img *VideoFrame, err error) {
	ff := &self.ff.ff

	cgotimg := C.int(0)
	frame := C.av_frame_alloc()
	cerr := C.wrap_avcodec_decode_video2(ff.codecCtx, frame, unsafe.Pointer(&pkt[0]), C.int(len(pkt)), &cgotimg)

	if cerr < C.int(0) {
		err = fmt.Errorf("ffmpeg: avcodec_decode_video2 failed: %d", cerr)
		return
	}

	if cgotimg != C.int(0) {
		w := int(frame.width)
		h := int(frame.height)
		ys := int(frame.linesize[0])
		cs := int(frame.linesize[1])

		img = &VideoFrame{
			Image: image.YCbCr{
				Y:              fromCPtr(unsafe.Pointer(frame.data[0]), ys*h),
				Cb:             fromCPtr(unsafe.Pointer(frame.data[1]), cs*h/2),
				Cr:             fromCPtr(unsafe.Pointer(frame.data[2]), cs*h/2),
				YStride:        ys,
				CStride:        cs,
				SubsampleRatio: image.YCbCrSubsampleRatio420,
				Rect:           image.Rect(0, 0, w, h),
			},
			frame: frame,
		}
		runtime.SetFinalizer(img, freeVideoFrame)
	}
	return
}

func NewVideoDecoder(name string) (dec *VideoDecoder, err error) {
	_dec := &VideoDecoder{}

	codec := C.avcodec_find_decoder_by_name(C.CString(name))
	if codec == nil || C.avcodec_get_type(codec.id) != C.AVMEDIA_TYPE_VIDEO {
		err = fmt.Errorf("ffmpeg: cannot find video decoder codecId=%d", codec.id)
		return
	}

	if _dec.ff, err = newFFCtxByCodec(codec); err != nil {
		return
	}
	if err = _dec.Setup(); err != nil {
		return
	}

	dec = _dec
	return
}

type VideoEncoder struct {
	ff        *ffctx
	Bitrate   int
	Width     int
	Height    int
	Gopsize   int
	Framesize int
}

func (self *VideoEncoder) SetBitrate(bitrate int) (err error) {
	self.Bitrate = bitrate
	return
}

func (self *VideoEncoder) SetOption(key string, val interface{}) (err error) {
	ff := &self.ff.ff

	sval := fmt.Sprint(val)
	if key == "profile" {
		ff.profile = C.avcodec_profile_name_to_int(ff.codec, C.CString(sval))
		if ff.profile == C.FF_PROFILE_UNKNOWN {
			err = fmt.Errorf("ffmpeg: profile `%s` invalid", sval)
			return
		}
		return
	}

	C.av_dict_set(&ff.options, C.CString(key), C.CString(sval), 0)
	return
}

func (self *VideoEncoder) GetOption(key string, val interface{}) (err error) {
	ff := &self.ff.ff
	entry := C.av_dict_get(ff.options, C.CString(key), nil, 0)
	if entry == nil {
		err = fmt.Errorf("ffmpeg: GetOption failed: `%s` not exists", key)
		return
	}
	switch p := val.(type) {
	case *string:
		*p = C.GoString(entry.value)
	case *int:
		fmt.Sscanf(C.GoString(entry.value), "%d", p)
	default:
		err = fmt.Errorf("ffmpeg: GetOption failed: val must be *string or *int receiver")
		return
	}
	return
}

func (self *VideoEncoder) Setup() (err error) {
	ff := &self.ff.ff

	ff.codecCtx.bit_rate = C.int64_t(self.Bitrate)

	if C.avcodec_open2(ff.codecCtx, ff.codec, nil) != 0 {
		err = fmt.Errorf("ffmpeg: encoder: avcodec_open2 failed")
		return
	}

	ff.frame = C.av_frame_alloc()
	return
}

func (self *VideoEncoder) Encode(frame *VideoFrame) (gotpkt bool,pkt []byte, err error) {

	ff := &self.ff.ff

	cpkt := C.AVPacket{}
	cgotpkt := C.int(0)

	//todo  assign go's frame to c's frame

	cerr := C.avcodec_encode_video2(ff.codecCtx, &cpkt, ff.frame, &cgotpkt)
	if cerr < C.int(0) {
		err = fmt.Errorf("ffmpeg: avcodec_encode_video2 failed: %d", cerr)
		return
	}

	if cgotpkt != 0  {
		gotpkt = true
		pkt = C.GoBytes(unsafe.Pointer(cpkt.data), cpkt.size)
		C.av_packet_unref(&cpkt)
	}
	return
}


func (self *VideoEncoder) Close() {
	freeFFCtx(self.ff)
}

func NewVideoEncoder(name string) (enc *VideoEncoder, err error) {
	_enc := &VideoEncoder{}
	codec := C.avcodec_find_encoder_by_name(C.CString(name))
	if codec == nil || C.avcodec_get_type(codec.id) != C.AVMEDIA_TYPE_VIDEO {
		err = fmt.Errorf("ffmpeg: cannot find video encoder name=%s", name)
		return
	}

	if _enc.ff, err = newFFCtxByCodec(codec); err != nil {
		return
	}
	enc = _enc
	return
}
