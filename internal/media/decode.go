package media

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"io"
	"time"

	_ "golang.org/x/image/webp"
	_ "image/jpeg"
	_ "image/png"
)

// Frame holds one fully composed frame of an animated GIF.
type Frame struct {
	Image image.Image
	Delay time.Duration
}

// DecodeLimits bound source dimensions before a decoder can allocate the full
// pixel surface. Both limits are enforced when positive.
type DecodeLimits struct {
	MaxEncodedBytes int64
	MaxDimension    int
	MaxPixels       int64
}

// GIFLimits additionally bound frame count and aggregate composed-frame memory.
type GIFLimits struct {
	DecodeLimits
	MaxFrames      int
	MaxMemoryBytes int64
}

func defaultDecodeLimits() DecodeLimits {
	return DecodeLimits{MaxEncodedBytes: DefaultMaxResponseBytes, MaxDimension: DefaultMaxSourceDimension, MaxPixels: DefaultMaxSourcePixels}
}

func defaultGIFLimits() GIFLimits {
	return GIFLimits{
		DecodeLimits:   defaultDecodeLimits(),
		MaxFrames:      DefaultGIFMaxFrames,
		MaxMemoryBytes: DefaultGIFMaxMemoryBytes,
	}
}

// Decode decodes a still image with the package's bounded defaults.
func Decode(r io.Reader) (image.Image, error) { return DecodeWithLimits(r, defaultDecodeLimits()) }

// DecodeWithLimits inspects the source dimensions before decoding pixels. This
// prevents a small compressed image from causing an unexpectedly large decode.
func DecodeWithLimits(r io.Reader, limits DecodeLimits) (image.Image, error) {
	raw, err := readEncoded(r, limits.MaxEncodedBytes)
	if err != nil {
		return nil, fmt.Errorf("media: read image: %w", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("media: decode config: %w", err)
	}
	if err := validateDimensions(cfg.Width, cfg.Height, limits); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("media: decode: %w", err)
	}
	return img, nil
}

// DecodeGIF decodes and composes a GIF with bounded defaults.
func DecodeGIF(r io.Reader) ([]Frame, error) { return DecodeGIFWithLimits(r, defaultGIFLimits()) }

// DecodeGIFWithLimits validates canvas size, frame count, every frame's bounds,
// and aggregate RGBA snapshot memory before allocating the composition canvas.
// gif.DecodeAll necessarily materializes encoded paletted frames, but no full
// RGBA frame composition occurs until all limits pass.
func DecodeGIFWithLimits(r io.Reader, limits GIFLimits) ([]Frame, error) {
	raw, err := readEncoded(r, limits.MaxEncodedBytes)
	if err != nil {
		return nil, fmt.Errorf("media: read gif: %w", err)
	}
	prepared, err := preflightGIF(raw, limits)
	if err != nil {
		return nil, err
	}
	g, err := gif.DecodeAll(bytes.NewReader(prepared))
	if err != nil {
		return nil, fmt.Errorf("media: decode gif: %w", err)
	}
	if len(g.Image) == 0 {
		return nil, fmt.Errorf("media: decode gif: no frames")
	}

	width, height := g.Config.Width, g.Config.Height
	if width <= 0 || height <= 0 {
		b := g.Image[0].Bounds()
		width, height = b.Dx(), b.Dy()
	}
	if err := validateDimensions(width, height, limits.DecodeLimits); err != nil {
		return nil, fmt.Errorf("media: GIF canvas: %w", err)
	}
	canvasBounds := image.Rect(0, 0, width, height)
	pixels, _ := pixelCount(width, height)
	// One live canvas plus one independent RGBA snapshot per returned frame.
	composedBytes, ok := checkedMul(pixels, int64(4*(len(g.Image)+1)))
	if !ok || (limits.MaxMemoryBytes > 0 && composedBytes > limits.MaxMemoryBytes) {
		return nil, fmt.Errorf("media: GIF composed memory exceeds %d bytes", limits.MaxMemoryBytes)
	}
	var encodedFrameBytes int64
	for i, src := range g.Image {
		if src == nil || !src.Bounds().In(canvasBounds) {
			return nil, fmt.Errorf("media: GIF frame %d bounds %v exceed canvas %v", i, src.Bounds(), canvasBounds)
		}
		encodedFrameBytes += int64(len(src.Pix))
		if encodedFrameBytes < 0 || (limits.MaxMemoryBytes > 0 && encodedFrameBytes+composedBytes > limits.MaxMemoryBytes) {
			return nil, fmt.Errorf("media: GIF aggregate memory exceeds %d bytes", limits.MaxMemoryBytes)
		}
	}

	canvas := image.NewRGBA(canvasBounds)
	if p, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(p) {
		draw.Draw(canvas, canvas.Bounds(), &image.Uniform{p[g.BackgroundIndex]}, image.Point{}, draw.Src)
	}

	frames := make([]Frame, 0, len(g.Image))
	for i, src := range g.Image {
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		delay := time.Duration(0)
		if i < len(g.Delay) {
			delay = time.Duration(g.Delay[i]) * 10 * time.Millisecond
		}
		draw.Draw(canvas, src.Bounds(), src, src.Bounds().Min, draw.Over)
		snapshot := image.NewRGBA(canvas.Bounds())
		draw.Draw(snapshot, snapshot.Bounds(), canvas, image.Point{}, draw.Src)
		frames = append(frames, Frame{Image: snapshot, Delay: delay})

		if disposal == 2 {
			bg := color.RGBA{}
			if p, ok := g.Config.ColorModel.(color.Palette); ok && int(g.BackgroundIndex) < len(p) {
				r32, gv, b32, a32 := p[g.BackgroundIndex].RGBA()
				bg = color.RGBA{R: uint8(r32 >> 8), G: uint8(gv >> 8), B: uint8(b32 >> 8), A: uint8(a32 >> 8)}
			}
			draw.Draw(canvas, src.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
		}
	}
	return frames, nil
}

// preflightGIF walks the bounded encoded stream without decoding pixels. It
// rejects invalid canvas/frame geometry and aggregate memory before DecodeAll,
// and truncates after MaxFrames with a valid trailer so the established
// first-N-frame animation cap does not allocate discarded source frames.
func preflightGIF(raw []byte, limits GIFLimits) ([]byte, error) {
	if len(raw) < 13 || (string(raw[:6]) != "GIF87a" && string(raw[:6]) != "GIF89a") {
		return nil, fmt.Errorf("media: decode gif: invalid header")
	}
	width := int(binary.LittleEndian.Uint16(raw[6:8]))
	height := int(binary.LittleEndian.Uint16(raw[8:10]))
	if err := validateDimensions(width, height, limits.DecodeLimits); err != nil {
		return nil, fmt.Errorf("media: GIF canvas: %w", err)
	}
	pos := 13
	packed := raw[10]
	if packed&0x80 != 0 {
		pos += 3 * (1 << ((packed & 0x07) + 1))
		if pos > len(raw) {
			return nil, fmt.Errorf("media: decode gif: truncated global color table")
		}
	}
	canvasPixels, _ := pixelCount(width, height)
	frameCount := 0
	var sourcePixels int64
	for pos < len(raw) {
		switch raw[pos] {
		case 0x3b:
			if frameCount == 0 {
				return nil, fmt.Errorf("media: decode gif: no frames")
			}
			return raw, nil
		case 0x21:
			// Extension introducer + label followed by data sub-blocks.
			pos += 2
			var err error
			pos, err = skipGIFSubBlocks(raw, pos)
			if err != nil {
				return nil, err
			}
		case 0x2c:
			if pos+10 > len(raw) {
				return nil, fmt.Errorf("media: decode gif: truncated image descriptor")
			}
			left := int(binary.LittleEndian.Uint16(raw[pos+1 : pos+3]))
			top := int(binary.LittleEndian.Uint16(raw[pos+3 : pos+5]))
			frameW := int(binary.LittleEndian.Uint16(raw[pos+5 : pos+7]))
			frameH := int(binary.LittleEndian.Uint16(raw[pos+7 : pos+9]))
			if frameW <= 0 || frameH <= 0 || left > width-frameW || top > height-frameH {
				return nil, fmt.Errorf("media: GIF frame %d exceeds canvas", frameCount)
			}
			framePixels, ok := pixelCount(frameW, frameH)
			if !ok {
				return nil, fmt.Errorf("media: GIF frame %d dimensions overflow", frameCount)
			}
			sourcePixels += framePixels
			frameCount++
			composed, ok := checkedMul(canvasPixels, int64(4*(frameCount+1)))
			if !ok || sourcePixels > 1<<63-1-composed || (limits.MaxMemoryBytes > 0 && sourcePixels+composed > limits.MaxMemoryBytes) {
				return nil, fmt.Errorf("media: GIF aggregate memory exceeds %d bytes", limits.MaxMemoryBytes)
			}
			localPacked := raw[pos+9]
			pos += 10
			if localPacked&0x80 != 0 {
				pos += 3 * (1 << ((localPacked & 0x07) + 1))
			}
			if pos >= len(raw) {
				return nil, fmt.Errorf("media: decode gif: truncated frame data")
			}
			pos++ // LZW minimum code size.
			var err error
			pos, err = skipGIFSubBlocks(raw, pos)
			if err != nil {
				return nil, err
			}
			if limits.MaxFrames > 0 && frameCount >= limits.MaxFrames {
				truncated := make([]byte, pos+1)
				copy(truncated, raw[:pos])
				truncated[pos] = 0x3b
				return truncated, nil
			}
		default:
			return nil, fmt.Errorf("media: decode gif: invalid block marker %#x", raw[pos])
		}
	}
	return nil, fmt.Errorf("media: decode gif: missing trailer")
}

func skipGIFSubBlocks(raw []byte, pos int) (int, error) {
	for {
		if pos >= len(raw) {
			return 0, fmt.Errorf("media: decode gif: truncated data blocks")
		}
		size := int(raw[pos])
		pos++
		if size == 0 {
			return pos, nil
		}
		if size > len(raw)-pos {
			return 0, fmt.Errorf("media: decode gif: truncated data block")
		}
		pos += size
	}
}

func readEncoded(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseBytes
	}
	readLimit := maxBytes + 1
	if readLimit <= 0 { // int64 overflow from an explicitly maximal setting.
		readLimit = maxBytes
	}
	raw, err := io.ReadAll(io.LimitReader(r, readLimit))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, fmt.Errorf("encoded media exceeds %d bytes", maxBytes)
	}
	return raw, nil
}

func validateDimensions(width, height int, limits DecodeLimits) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("media: invalid image dimensions %dx%d", width, height)
	}
	if limits.MaxDimension > 0 && (width > limits.MaxDimension || height > limits.MaxDimension) {
		return fmt.Errorf("media: image dimensions %dx%d exceed %d", width, height, limits.MaxDimension)
	}
	pixels, ok := pixelCount(width, height)
	if !ok || (limits.MaxPixels > 0 && pixels > limits.MaxPixels) {
		return fmt.Errorf("media: image has %d pixels, limit is %d", pixels, limits.MaxPixels)
	}
	return nil
}

func pixelCount(width, height int) (int64, bool) { return checkedMul(int64(width), int64(height)) }

func checkedMul(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || (a != 0 && b > (1<<63-1)/a) {
		return 0, false
	}
	return a * b, true
}
