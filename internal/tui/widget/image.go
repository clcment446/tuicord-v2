package widget

import (
	"container/list"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"awesomeProject/internal/tui/layout"
	"awesomeProject/internal/tui/screen"
	"awesomeProject/internal/tui/text"
	"awesomeProject/internal/tui/tui"
)

const kittyCacheBudget = 50 << 20

// ImageMode selects how an Image is rendered.
type ImageMode int

const (
	// ImageUnicode draws colored upper-half block cells.
	ImageUnicode ImageMode = iota
	// ImageASCII draws a grayscale ASCII ramp.
	ImageASCII
	// ImageKitty emits Kitty graphics protocol bytes through ANSI. Some callers
	// refer to this path as Kitty IPC because images are uploaded to the
	// terminal and then placed by ID.
	ImageKitty
	// ImageSixel emits Sixel bytes through ANSI.
	ImageSixel
)

// Image draws an image using a cell fallback and can emit terminal graphics.
type Image struct {
	path        string
	img         image.Image
	mode        ImageMode
	id          uint32
	placementID uint32
	pixelWidth  int
	pixelHeight int
	z           int
	style       screen.Style
	node        layout.Node
	err         error
}

type kittyUploadEntry struct {
	key  string
	data []byte
	size int
}

type kittyUploadCache struct {
	mu      sync.Mutex
	maxSize int
	size    int
	items   map[string]*list.Element
	order   *list.List
}

// KittyOptions controls high-resolution Kitty graphics output.
type KittyOptions struct {
	// PixelWidth and PixelHeight are the transmitted image size in pixels. When
	// omitted, the source image dimensions are used.
	PixelWidth, PixelHeight int
	// SourceX, SourceY, SourceWidth, and SourceHeight select the source
	// rectangle, in transmitted-image pixels, used for placement. Zero
	// SourceWidth or SourceHeight displays the whole transmitted image.
	SourceX, SourceY, SourceWidth, SourceHeight int
	// CellWidth and CellHeight are the terminal-cell area used for placement.
	// When omitted, PixelWidth and PixelHeight are used for compatibility with
	// ANSI.
	CellWidth, CellHeight int
	// X and Y are zero-based terminal-cell coordinates used when MoveCursor is
	// true. Kitty places images at the current cursor.
	X, Y int
	// PlacementID identifies this on-screen placement. When zero, the image ID
	// is used. Multiple placements can share one uploaded image ID.
	PlacementID uint32
	// MoveCursor emits a cursor movement before the placement.
	MoveCursor bool
	// Z sets the Kitty z-index. Zero is Kitty's default plane.
	Z int
}

var (
	nextImageID atomic.Uint32
	kittyCache  = newKittyUploadCache(kittyCacheBudget)
)

// NewImage returns an image widget that loads path on demand.
func NewImage(path string) *Image {
	return &Image{path: path, mode: ImageUnicode, id: newImageID(), node: layout.Node{Grow: 1}}
}

// NewImageFrom returns an image widget backed by img.
func NewImageFrom(img image.Image) *Image {
	return &Image{img: img, mode: ImageUnicode, id: newImageID(), node: layout.Node{Grow: 1}}
}

// NewKittyImage returns an image widget that renders through Kitty graphics.
func NewKittyImage(path string) *Image {
	return NewImage(path).SetMode(ImageKitty)
}

// NewKittyImageFrom returns an image widget backed by img that renders through
// Kitty graphics.
func NewKittyImageFrom(img image.Image) *Image {
	return NewImageFrom(img).SetMode(ImageKitty)
}

// SetMode sets the render mode and returns w.
func (w *Image) SetMode(mode ImageMode) *Image {
	if w == nil {
		return nil
	}
	w.mode = mode
	return w
}

// SetID sets the stable terminal image ID used by Kitty mode. ID zero renders
// transiently without caching.
func (w *Image) SetID(id uint32) *Image {
	if w == nil {
		return nil
	}
	w.id = id
	return w
}

// SetPlacementID sets the stable terminal placement ID used by Kitty mode. When
// unset, the image ID is also used as the placement ID.
func (w *Image) SetPlacementID(id uint32) *Image {
	if w == nil {
		return nil
	}
	w.placementID = id
	return w
}

// SetPixelSize sets the transmitted image payload size for terminal graphics.
// Zero dimensions use a terminal-cell based default.
func (w *Image) SetPixelSize(width, height int) *Image {
	if w == nil {
		return nil
	}
	w.pixelWidth = maxInt(width, 0)
	w.pixelHeight = maxInt(height, 0)
	return w
}

// SetZ sets the Kitty z-index used by terminal graphics rendering.
func (w *Image) SetZ(z int) *Image {
	if w == nil {
		return nil
	}
	w.z = z
	return w
}

// SetStyle sets the placeholder style used when decoding fails and returns w.
func (w *Image) SetStyle(style screen.Style) *Image {
	if w == nil {
		return nil
	}
	w.style = style
	return w
}

// Err returns the last image loading error, if any.
func (w *Image) Err() error {
	if w == nil {
		return nil
	}
	return w.err
}

// Measure returns the requested image area clamped to avail.
func (w *Image) Measure(avail tui.Size) tui.Size {
	if w == nil {
		return tui.Size{}
	}
	img := w.image()
	if img == nil {
		return tui.Size{W: minInt(maxInt(text.Width(w.path), 1), maxInt(avail.W, 1)), H: 1}
	}
	b := img.Bounds()
	width := b.Dx()
	height := maxInt((b.Dy()+1)/2, 1)
	if avail.W > 0 {
		width = minInt(width, avail.W)
	}
	if avail.H > 0 {
		height = minInt(height, avail.H)
	}
	return tui.Size{W: maxInt(width, 1), H: maxInt(height, 1)}
}

// Layout returns the image layout node.
func (w *Image) Layout() *layout.Node {
	if w == nil {
		return nil
	}
	return &w.node
}

// Draw renders the image with a cell fallback. Kitty and Sixel modes also attach
// terminal graphics bytes to the frame, so images behave like regular widgets
// in retained layouts.
func (w *Image) Draw(r screen.Region) {
	if w == nil {
		return
	}
	clear(r, w.style)
	img := w.image()
	if img == nil {
		drawPaddedText(r, 0, "[image: "+w.path+"]", w.style)
		return
	}
	if w.mode == ImageASCII {
		drawASCIIImage(r, img, w.style)
		return
	}
	if w.mode == ImageKitty || w.mode == ImageSixel {
		w.drawTerminalGraphic(r, img)
		return
	}
	drawUnicodeImage(r, img)
}

// Handle ignores input events and reports them unconsumed.
func (w *Image) Handle(tui.Event) bool {
	return false
}

// ANSI returns terminal graphics bytes for Kitty or Sixel modes. Most callers
// should render the Image as a widget; this low-level helper remains available
// for protocol tests and custom terminal output.
func (w *Image) ANSI(width, height int) ([]byte, error) {
	if w == nil {
		return nil, nil
	}
	img := w.image()
	if img == nil {
		return nil, w.err
	}
	if width <= 0 {
		width = img.Bounds().Dx()
	}
	if height <= 0 {
		height = img.Bounds().Dy()
	}
	switch w.mode {
	case ImageKitty:
		return kittyImage(img, KittyOptions{
			PixelWidth:  width,
			PixelHeight: height,
			CellWidth:   width,
			CellHeight:  height,
		}, w.id)
	case ImageSixel:
		return sixelImage(img, width, height), nil
	default:
		return nil, nil
	}
}

// KittyANSI returns Kitty graphics protocol bytes with independent pixel and
// terminal-cell placement dimensions. Most callers should use NewKittyImage or
// SetMode(ImageKitty), which places the image from Draw using normal layout.
func (w *Image) KittyANSI(opts KittyOptions) ([]byte, error) {
	if w == nil {
		return nil, nil
	}
	img := w.image()
	if img == nil {
		return nil, w.err
	}
	return kittyImage(img, opts, w.id)
}

func (w *Image) drawTerminalGraphic(r screen.Region, img image.Image) {
	visible := r.VisibleBounds()
	if visible.W <= 0 || visible.H <= 0 {
		return
	}
	bounds := r.Bounds()
	fullCellsW := maxInt(bounds.W, 1)
	fullCellsH := maxInt(bounds.H, 1)
	fullPixelW := w.terminalPixelWidth(img, fullCellsW)
	fullPixelH := w.terminalPixelHeight(img, fullCellsH)
	sourceX0 := (visible.X - bounds.X) * fullPixelW / fullCellsW
	sourceY0 := (visible.Y - bounds.Y) * fullPixelH / fullCellsH
	sourceX1 := (visible.X - bounds.X + visible.W) * fullPixelW / fullCellsW
	sourceY1 := (visible.Y - bounds.Y + visible.H) * fullPixelH / fullCellsH
	sourceW := maxInt(sourceX1-sourceX0, 1)
	sourceH := maxInt(sourceY1-sourceY0, 1)
	switch w.mode {
	case ImageKitty:
		payloadKey, upload, err := w.kittyUpload(img, fullPixelW, fullPixelH)
		if err != nil {
			w.err = err
			return
		}
		placement := kittyPlace(KittyOptions{
			SourceX:      sourceX0,
			SourceY:      sourceY0,
			SourceWidth:  sourceW,
			SourceHeight: sourceH,
			CellWidth:    visible.W,
			CellHeight:   visible.H,
			X:            visible.X,
			Y:            visible.Y,
			PlacementID:  w.effectivePlacementID(),
			MoveCursor:   true,
			Z:            w.z,
		}, w.id)
		r.AddGraphic(screen.Graphic{
			Key:        w.graphicKey("kitty"),
			PayloadKey: payloadKey,
			Clear:      kittyDeletePlacement(w.id, w.effectivePlacementID()),
			Free:       kittyDeleteImage(w.id),
			Upload:     upload,
			Data:       placement,
		})
	case ImageSixel:
		cropped := cropImage(img, sourceX0, sourceY0, sourceW, sourceH, fullPixelW, fullPixelH)
		width := maxInt(sourceW, 1)
		height := maxInt(sourceH, 1)
		ansi := append(cursorMove(visible.X, visible.Y), sixelImage(cropped, width, height)...)
		r.AddGraphic(screen.Graphic{
			Key:  w.graphicKey("sixel"),
			Data: ansi,
		})
	}
}

func (w *Image) terminalPixelWidth(img image.Image, cells int) int {
	if w.pixelWidth > 0 {
		return w.pixelWidth
	}
	if img != nil {
		if width := img.Bounds().Dx(); width > 0 {
			return width
		}
	}
	return maxInt(cells*18, 1)
}

func (w *Image) terminalPixelHeight(img image.Image, cells int) int {
	if w.pixelHeight > 0 {
		return w.pixelHeight
	}
	if img != nil {
		if height := img.Bounds().Dy(); height > 0 {
			return height
		}
	}
	return maxInt(cells*36, 1)
}

func (w *Image) graphicKey(protocol string) string {
	if w.id != 0 {
		if placementID := w.effectivePlacementID(); placementID != w.id {
			return fmt.Sprintf("%s:%d:%d", protocol, w.id, placementID)
		}
		return fmt.Sprintf("%s:%d", protocol, w.id)
	}
	return fmt.Sprintf("%s:%p", protocol, w)
}

func (w *Image) effectivePlacementID() uint32 {
	if w == nil {
		return 0
	}
	if w.placementID != 0 {
		return w.placementID
	}
	return w.id
}

func (w *Image) kittyUpload(img image.Image, width, height int) (string, []byte, error) {
	key := fmt.Sprintf("%d:%d:%d:%s:%p", w.id, width, height, w.path, img)
	upload, err := kittyCache.Get(key, func() ([]byte, error) {
		return kittyUpload(img, width, height, w.id)
	})
	return key, upload, err
}

func newImageID() uint32 {
	return nextImageID.Add(1)
}

func (w *Image) image() image.Image {
	if w == nil {
		return nil
	}
	if w.img != nil || w.path == "" {
		return w.img
	}
	f, err := os.Open(w.path)
	if err != nil {
		w.err = err
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		w.err = err
		return nil
	}
	w.img = img
	w.err = nil
	return img
}

func drawUnicodeImage(r screen.Region, img image.Image) {
	for y := 0; y < r.Height(); y++ {
		for x := 0; x < r.Width(); x++ {
			top := sample(img, x, y*2, r.Width(), r.Height()*2)
			bottom := sample(img, x, y*2+1, r.Width(), r.Height()*2)
			r.Set(x, y, screen.Cell{
				Content: "▀",
				Style: screen.Style{
					Fg: screen.RGB(top.R, top.G, top.B),
					Bg: screen.RGB(bottom.R, bottom.G, bottom.B),
				},
			})
		}
	}
}

func drawASCIIImage(r screen.Region, img image.Image, style screen.Style) {
	const ramp = " .:-=+*#%@"
	for y := 0; y < r.Height(); y++ {
		for x := 0; x < r.Width(); x++ {
			c := sample(img, x, y, r.Width(), r.Height())
			luma := int(c.R)*299 + int(c.G)*587 + int(c.B)*114
			idx := luma * (len(ramp) - 1) / (255 * 1000)
			r.Set(x, y, styled(ramp[idx:idx+1], style))
		}
	}
}

func sample(img image.Image, x, y, w, h int) color.RGBA {
	b := img.Bounds()
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	px := b.Min.X + x*b.Dx()/w
	py := b.Min.Y + y*b.Dy()/h
	r, g, bl, a := img.At(px, py).RGBA()
	if a == 0 {
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(bl >> 8), A: uint8(a >> 8)}
}

func kittyImage(img image.Image, opts KittyOptions, id uint32) ([]byte, error) {
	b := img.Bounds()
	width := opts.PixelWidth
	height := opts.PixelHeight
	if width <= 0 {
		width = b.Dx()
	}
	if height <= 0 {
		height = b.Dy()
	}
	opts.PixelWidth = width
	opts.PixelHeight = height
	upload, err := kittyUpload(img, width, height, id)
	if err != nil {
		return nil, err
	}
	placement := kittyPlace(opts, id)
	out := make([]byte, 0, len(upload)+len(placement))
	out = append(out, upload...)
	out = append(out, placement...)
	return out, nil
}

func kittyUpload(img image.Image, width, height int, id uint32) ([]byte, error) {
	b := img.Bounds()
	if width <= 0 {
		width = b.Dx()
	}
	if height <= 0 {
		height = b.Dy()
	}

	scaled := scaleNRGBA(img, width, height)
	if b.Dx() == width && b.Dy() == height {
		scaled = tightNRGBA(img)
	}
	payload := base64.StdEncoding.EncodeToString(scaled.Pix)
	var out strings.Builder
	for i := 0; i < len(payload); i += 4096 {
		end := minInt(i+4096, len(payload))
		more := 0
		if end < len(payload) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(&out, "\x1b_Ga=t,q=2,i=%d,f=32,s=%d,v=%d,t=d,m=%d;%s\x1b\\", id, width, height, more, payload[i:end])
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, payload[i:end])
		}
	}
	return []byte(out.String()), nil
}

func kittyPlace(opts KittyOptions, id uint32) []byte {
	placementID := opts.PlacementID
	if placementID == 0 {
		placementID = id
	}
	cellWidth := opts.CellWidth
	cellHeight := opts.CellHeight
	if cellWidth <= 0 {
		cellWidth = opts.PixelWidth
	}
	if cellHeight <= 0 {
		cellHeight = opts.PixelHeight
	}
	cellWidth = maxInt(cellWidth, 1)
	cellHeight = maxInt(cellHeight, 1)

	var out strings.Builder
	if opts.MoveCursor {
		fmt.Fprintf(&out, "\x1b[%d;%dH", maxInt(opts.Y, 0)+1, maxInt(opts.X, 0)+1)
	}
	fmt.Fprintf(&out, "\x1b_Ga=p,q=2,i=%d,p=%d", id, placementID)
	if opts.SourceWidth > 0 && opts.SourceHeight > 0 {
		fmt.Fprintf(&out, ",x=%d,y=%d,w=%d,h=%d", maxInt(opts.SourceX, 0), maxInt(opts.SourceY, 0), opts.SourceWidth, opts.SourceHeight)
	}
	fmt.Fprintf(&out, ",c=%d,r=%d,z=%d\x1b\\", cellWidth, cellHeight, opts.Z)
	return []byte(out.String())
}

func kittyDeletePlacement(id, placementID uint32) []byte {
	if id == 0 {
		return nil
	}
	if placementID == 0 {
		placementID = id
	}
	return []byte(fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,p=%d\x1b\\", id, placementID))
}

func kittyDeleteImage(id uint32) []byte {
	if id == 0 {
		return nil
	}
	return []byte(fmt.Sprintf("\x1b_Ga=d,d=I,i=%d\x1b\\", id))
}

func cursorMove(x, y int) []byte {
	return []byte(fmt.Sprintf("\x1b[%d;%dH", maxInt(y, 0)+1, maxInt(x, 0)+1))
}

func newKittyUploadCache(maxSize int) *kittyUploadCache {
	return &kittyUploadCache{
		maxSize: maxInt(maxSize, 0),
		items:   map[string]*list.Element{},
		order:   list.New(),
	}
}

func (c *kittyUploadCache) Get(key string, build func() ([]byte, error)) ([]byte, error) {
	if c == nil {
		return build()
	}
	c.mu.Lock()
	if elem := c.items[key]; elem != nil {
		c.order.MoveToFront(elem)
		data := elem.Value.(*kittyUploadEntry).data
		c.mu.Unlock()
		return data, nil
	}
	c.mu.Unlock()

	data, err := build()
	if err != nil || len(data) == 0 {
		return data, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if elem := c.items[key]; elem != nil {
		c.order.MoveToFront(elem)
		return elem.Value.(*kittyUploadEntry).data, nil
	}
	size := len(data)
	if c.maxSize > 0 && size > c.maxSize {
		return data, nil
	}
	entry := &kittyUploadEntry{key: key, data: data, size: size}
	c.items[key] = c.order.PushFront(entry)
	c.size += size
	c.evict()
	return data, nil
}

func (c *kittyUploadCache) evict() {
	for c.maxSize > 0 && c.size > c.maxSize {
		elem := c.order.Back()
		if elem == nil {
			return
		}
		entry := elem.Value.(*kittyUploadEntry)
		delete(c.items, entry.key)
		c.size -= entry.size
		c.order.Remove(elem)
	}
}

func sixelImage(img image.Image, width, height int) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "\x1bPq\"1;1;%d;%d#0;2;0;0;0#1;2;100;100;100#0", width, height)
	for y := 0; y < height; y += 6 {
		if y > 0 {
			out.WriteByte('-')
		}
		for x := 0; x < width; x++ {
			bits := 0
			for bit := 0; bit < 6; bit++ {
				if y+bit >= height {
					continue
				}
				c := sample(img, x, y+bit, width, height)
				luma := int(c.R)*299 + int(c.G)*587 + int(c.B)*114
				if luma < 128*1000 {
					bits |= 1 << bit
				}
			}
			out.WriteByte(byte(63 + bits))
		}
	}
	out.WriteString("\x1b\\")
	return []byte(out.String())
}

func scaleRGBA(img image.Image, width, height int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.SetRGBA(x, y, sample(img, x, y, width, height))
		}
	}
	return out
}

func scaleNRGBA(img image.Image, width, height int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			out.SetNRGBA(x, y, sampleNRGBA(img, x, y, width, height))
		}
	}
	return out
}

func sampleNRGBA(img image.Image, x, y, w, h int) color.NRGBA {
	b := img.Bounds()
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	px := b.Min.X + x*b.Dx()/w
	py := b.Min.Y + y*b.Dy()/h
	return nrgba(img.At(px, py))
}

func nrgba(c color.Color) color.NRGBA {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return color.NRGBA{}
	}
	return color.NRGBA{
		R: uint8((r * 0xffff / a) >> 8),
		G: uint8((g * 0xffff / a) >> 8),
		B: uint8((b * 0xffff / a) >> 8),
		A: uint8(a >> 8),
	}
}

func cropImage(img image.Image, x, y, width, height, fullWidth, fullHeight int) image.Image {
	if img == nil {
		return nil
	}
	if width <= 0 || height <= 0 {
		return img
	}
	scaled := scaleRGBA(img, maxInt(fullWidth, 1), maxInt(fullHeight, 1))
	rect := image.Rect(x, y, x+width, y+height).Intersect(scaled.Bounds())
	if rect.Empty() {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(out, out.Bounds(), scaled, rect.Min, draw.Src)
	return out
}

func tightRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok &&
		rgba.Rect.Min.X == 0 &&
		rgba.Rect.Min.Y == 0 &&
		rgba.Stride == rgba.Rect.Dx()*4 &&
		len(rgba.Pix) == rgba.Stride*rgba.Rect.Dy() {
		return rgba
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

func tightNRGBA(img image.Image) *image.NRGBA {
	if nrgba, ok := img.(*image.NRGBA); ok &&
		nrgba.Rect.Min.X == 0 &&
		nrgba.Rect.Min.Y == 0 &&
		nrgba.Stride == nrgba.Rect.Dx()*4 &&
		len(nrgba.Pix) == nrgba.Stride*nrgba.Rect.Dy() {
		return nrgba
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

func init() {
	image.RegisterFormat("gif", "GIF8?a", gif.Decode, gif.DecodeConfig)
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
}
