package vcodec

/*
#cgo LDFLAGS: -lavformat -lavutil -lavcodec -lswscale
#cgo CFLAGS: -Wno-deprecated
#include <libavformat/avformat.h>
#include <libavcodec/avcodec.h>
#include <libavutil/avutil.h>
#include <libavutil/imgutils.h>
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

void vffinit() {
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
	"reflect"
	"runtime"
	"unsafe"
)

func init() {
	C.vffinit()
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
	Width  int
	Height int
	Data   []byte
	frame  *C.AVFrame
}

func (self *VideoFrame) Free() {
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
		width := int(frame.width)
		height := int(frame.height)

		stride_y := int(frame.linesize[0])
		stride_u := int(frame.linesize[1])
		stride_v := int(frame.linesize[2])

		data := make([]byte,width * height * 3 / 2)

		nYUVBufsize := 0
		nVOffset := 0

		for i := 0; i < height; i++ {
			copy(data[nYUVBufsize:],fromCPtr(unsafe.Pointer(uintptr(unsafe.Pointer(frame.data[0])) + uintptr(i * stride_y)), width))
			nYUVBufsize += width
		}
		for i := 0; i < height / 2; i++ {
			copy(data[nYUVBufsize:], fromCPtr(unsafe.Pointer(uintptr(unsafe.Pointer(frame.data[1])) + uintptr(i * stride_u)), width / 2))
			nYUVBufsize += width / 2
			copy(data[width * height * 5 / 4 + nVOffset:], fromCPtr(unsafe.Pointer(uintptr(unsafe.Pointer(frame.data[2])) + uintptr(i * stride_v)), width / 2))
			nVOffset += width / 2
		}

		img = &VideoFrame{
			Data:   data,
			Height: height,
			Width:  width,
			frame:  frame,
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
	Framerate int
}

func (self *VideoEncoder) SetBitrate(bitrate int) (err error) {
	self.Bitrate = bitrate
	return
}

func (self *VideoEncoder) SetHeight(height int) (err error) {
	self.Height = height
	return
}

func (self *VideoEncoder) SetWidth(width int) (err error) {
	self.Width = width
	return
}

func (self *VideoEncoder) SetGopsize(gop int) (err error) {
	self.Gopsize = gop
	return
}

func (self *VideoEncoder) SetFramerate(framerate int) (err error) {
	self.Framerate = framerate
	return
}

func (self *VideoEncoder) SetOption(key string, val interface{}) (err error) {
	ff := &self.ff.ff

	sval := fmt.Sprint(val)
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
	ff.codecCtx.width = C.int(self.Width)
	ff.codecCtx.height = C.int(self.Height)

	ff.codecCtx.pix_fmt = C.AV_PIX_FMT_YUV420P
	ff.codecCtx.gop_size = C.int(self.Gopsize)

	ff.codecCtx.time_base.num = 1
	ff.codecCtx.time_base.den = C.int(self.Framerate)

	if C.avcodec_open2(ff.codecCtx, ff.codec, &ff.options) != 0 {
		err = fmt.Errorf("ffmpeg: encoder: avcodec_open2 failed")
		return
	}

	ff.frame = C.av_frame_alloc()
	cerr := C.av_image_alloc(&ff.frame.data[0], &ff.frame.linesize[0], ff.codecCtx.width, ff.codecCtx.height, ff.codecCtx.pix_fmt, 32)
	if cerr < C.int(0) {
		err = fmt.Errorf("ffmpeg: av_image_alloc failed: %d",cerr)
		return
	}

	return
}

func (self *VideoEncoder) Encode(frame *VideoFrame) (gotpkt bool, pkt []byte, err error) {

	ff := &self.ff.ff

	cpkt := C.AVPacket{}
	cgotpkt := C.int(0)

	videoFrameAssignToFF(frame, ff.frame)

	cerr := C.avcodec_encode_video2(ff.codecCtx, &cpkt, ff.frame, &cgotpkt)
	if cerr < C.int(0) {
		err = fmt.Errorf("ffmpeg: avcodec_encode_video2 failed: %d", cerr)
		return
	}

	if cgotpkt != 0 {
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

func videoFrameAssignToFF(frame *VideoFrame, f *C.AVFrame) {

	f.width = C.int(frame.Width)
	f.height = C.int(frame.Height)
	f.format = C.AV_PIX_FMT_YUV420P

	// Y
	f.data[0] = (*C.uint8_t)(unsafe.Pointer(&frame.Data[0]))
	// U
	f.data[1] = (*C.uint8_t)(unsafe.Pointer(&frame.Data[frame.Width * frame.Height]))
	// V
	f.data[2] = (*C.uint8_t)(unsafe.Pointer(&frame.Data[frame.Width * frame.Height * 5/4]))

	f.linesize[0] = f.width
	f.linesize[1] = f.width >> 1
	f.linesize[2] = f.width >> 1
}
