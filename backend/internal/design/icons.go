package design

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Node types for decoration: a line icon and a decorative shape (section
// dividers, blobs). Their SVG comes from the built-in sets below, never from
// the design document, so a design cannot inject markup through them.
const (
	TypeIcon  = "icon"
	TypeShape = "shape"
)

// Icon is a 24×24 icon: SVG elements drawn with stroke="currentColor"
// (Fill: drawn with fill="currentColor" instead, e.g. brand marks).
type Icon struct {
	Body string `json:"body"`
	Fill bool   `json:"fill,omitempty"`
}

// Shape is a decorative SVG filled with currentColor. Stretch shapes
// (dividers) fill their box exactly; the others keep their proportions.
type Shape struct {
	ViewBox string `json:"viewBox"`
	Body    string `json:"body"`
	Stretch bool   `json:"stretch,omitempty"`
	Kind    string `json:"kind"` // divider | blob
}

// Icons is the built-in icon set (drawn for Anasaki Studio).
var Icons = map[string]Icon{
	"check":         {Body: `<path d="M5 12.5l4.5 4.5L19 7.5"/>`},
	"x":             {Body: `<path d="M6 6l12 12M18 6L6 18"/>`},
	"plus":          {Body: `<path d="M12 5v14M5 12h14"/>`},
	"minus":         {Body: `<path d="M5 12h14"/>`},
	"arrow-right":   {Body: `<path d="M5 12h14M13 6l6 6-6 6"/>`},
	"arrow-left":    {Body: `<path d="M19 12H5M11 6l-6 6 6 6"/>`},
	"arrow-up":      {Body: `<path d="M12 19V5M6 11l6-6 6 6"/>`},
	"arrow-down":    {Body: `<path d="M12 5v14M6 13l6 6 6-6"/>`},
	"chevron-right": {Body: `<path d="M9 6l6 6-6 6"/>`},
	"chevron-left":  {Body: `<path d="M15 6l-6 6 6 6"/>`},
	"chevron-down":  {Body: `<path d="M6 9l6 6 6-6"/>`},
	"chevron-up":    {Body: `<path d="M6 15l6-6 6 6"/>`},
	"external":      {Body: `<path d="M14 4h6v6M20 4l-9 9M18 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h5"/>`},
	"star":          {Body: `<path d="M12 3.5l2.6 5.3 5.9.9-4.25 4.1 1 5.8L12 16.9l-5.25 2.7 1-5.8L3.5 9.7l5.9-.9z"/>`},
	"heart":         {Body: `<path d="M12 20s-7-4.4-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 10c0 5.6-7 10-7 10z"/>`},
	"home":          {Body: `<path d="M4 11l8-7 8 7"/><path d="M6 9.5V20h12V9.5"/><path d="M10 20v-5h4v5"/>`},
	"mail":          {Body: `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3.5 6.5l8.5 6.5 8.5-6.5"/>`},
	"phone":         {Body: `<path d="M5 4h4l2 5-2.5 1.5a11 11 0 0 0 5 5L15 13l5 2v4a1 1 0 0 1-1 1A16 16 0 0 1 4 5a1 1 0 0 1 1-1z"/>`},
	"map-pin":       {Body: `<path d="M12 21s-6.5-6.2-6.5-11.5a6.5 6.5 0 0 1 13 0C18.5 14.8 12 21 12 21z"/><circle cx="12" cy="9.5" r="2.5"/>`},
	"clock":         {Body: `<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`},
	"calendar":      {Body: `<rect x="3" y="5" width="18" height="16" rx="2"/><path d="M3 10h18M8 3v4M16 3v4"/>`},
	"user":          {Body: `<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>`},
	"users":         {Body: `<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20a6.5 6.5 0 0 1 13 0"/><path d="M16 4.6a3.5 3.5 0 0 1 0 6.8M18 14a6.5 6.5 0 0 1 3.5 6"/>`},
	"search":        {Body: `<circle cx="11" cy="11" r="7"/><path d="M20 20l-4-4"/>`},
	"menu":          {Body: `<path d="M4 6h16M4 12h16M4 18h16"/>`},
	"cart":          {Body: `<circle cx="9" cy="20" r="1.5"/><circle cx="18" cy="20" r="1.5"/><path d="M2 3h3l2.6 12.4a1 1 0 0 0 1 .8h9.7a1 1 0 0 0 1-.8L21 7H6"/>`},
	"bag":           {Body: `<path d="M5 8h14l-1 13H6z"/><path d="M9 8V6a3 3 0 0 1 6 0v2"/>`},
	"gift":          {Body: `<rect x="3" y="8" width="18" height="4" rx="1"/><path d="M5 12v9h14v-9M12 8v13M12 8S10 3 7.5 4.5 9 8 12 8zM12 8s2-5 4.5-3.5S15 8 12 8z"/>`},
	"globe":         {Body: `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>`},
	"link":          {Body: `<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>`},
	"play":          {Body: `<path d="M7 4.5v15l12-7.5z"/>`},
	"pause":         {Body: `<path d="M8 5v14M16 5v14"/>`},
	"image":         {Body: `<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="9.5" r="1.8"/><path d="M21 16l-5-5L5 20"/>`},
	"camera":        {Body: `<path d="M4 8h3l2-3h6l2 3h3a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V9a1 1 0 0 1 1-1z"/><circle cx="12" cy="13.5" r="3.5"/>`},
	"video":         {Body: `<rect x="3" y="6" width="13" height="12" rx="2"/><path d="M16 10l5-3v10l-5-3"/>`},
	"music":         {Body: `<path d="M9 18V5l11-2v13"/><circle cx="6.5" cy="18" r="2.5"/><circle cx="17.5" cy="16" r="2.5"/>`},
	"lock":          {Body: `<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>`},
	"shield":        {Body: `<path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z"/><path d="M8.5 12l2.5 2.5 4.5-4.5"/>`},
	"zap":           {Body: `<path d="M13 2L4 14h7l-1 8 9-12h-7z"/>`},
	"sun":           {Body: `<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>`},
	"moon":          {Body: `<path d="M20 14.5A8 8 0 1 1 9.5 4a6.5 6.5 0 0 0 10.5 10.5z"/>`},
	"cloud":         {Body: `<path d="M7 18a4.5 4.5 0 0 1-.5-9A6 6 0 0 1 18 9.5a4.3 4.3 0 0 1-.5 8.5z"/>`},
	"leaf":          {Body: `<path d="M5 19C5 10 10 5 20 4c-1 10-6 15-15 15z"/><path d="M5 19l8-8"/>`},
	"coffee":        {Body: `<path d="M4 9h13v5a5 5 0 0 1-5 5H9a5 5 0 0 1-5-5z"/><path d="M17 10h1.5a2.5 2.5 0 0 1 0 5H17M8 3v3M12 3v3"/>`},
	"message":       {Body: `<path d="M4 5h16v11H9l-5 4z"/>`},
	"chat":          {Body: `<path d="M21 11.5a8.5 8.5 0 0 1-12.4 7.5L3 20.5l1.5-5A8.5 8.5 0 1 1 21 11.5z"/>`},
	"send":          {Body: `<path d="M21 3L10 14M21 3l-7 18-4-7-7-4z"/>`},
	"download":      {Body: `<path d="M12 4v11M7 10l5 5 5-5M5 20h14"/>`},
	"upload":        {Body: `<path d="M12 20V9M7 14l5-5 5 5M5 4h14"/>`},
	"sliders":       {Body: `<path d="M4 7h10M18 7h2M4 17h4M12 17h8"/><circle cx="16" cy="7" r="2"/><circle cx="10" cy="17" r="2"/>`},
	"bell":          {Body: `<path d="M6 16V11a6 6 0 0 1 12 0v5l2 2H4z"/><path d="M10 21h4"/>`},
	"bookmark":      {Body: `<path d="M6 3h12v18l-6-4-6 4z"/>`},
	"tag":           {Body: `<path d="M3 12V4a1 1 0 0 1 1-1h8l9 9-9 9z"/><circle cx="7.5" cy="7.5" r="1.5"/>`},
	"truck":         {Body: `<path d="M2 6h12v10H2zM14 10h4l3 3v3h-7"/><circle cx="6" cy="18" r="2"/><circle cx="17" cy="18" r="2"/>`},
	"credit-card":   {Body: `<rect x="2" y="5" width="20" height="14" rx="2"/><path d="M2 10h20M6 15h4"/>`},
	"award":         {Body: `<circle cx="12" cy="9" r="6"/><path d="M8.5 14L7 22l5-3 5 3-1.5-8"/>`},
	"thumbs-up":     {Body: `<path d="M7 10v11H3V10zM7 10l4-7a2 2 0 0 1 3 2l-1 5h6a2 2 0 0 1 2 2.3l-1.4 7A2 2 0 0 1 17.6 21H7"/>`},
	"smile":         {Body: `<circle cx="12" cy="12" r="9"/><path d="M8 14s1.5 2 4 2 4-2 4-2M9 9.5h.01M15 9.5h.01"/>`},
	"info":          {Body: `<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7.5h.01"/>`},
	"alert":         {Body: `<path d="M12 3L2 20h20z"/><path d="M12 10v4M12 17.5h.01"/>`},
	"help":          {Body: `<circle cx="12" cy="12" r="9"/><path d="M9.5 9a2.5 2.5 0 1 1 3.5 2.3c-.6.3-1 .9-1 1.5V14M12 17.5h.01"/>`},
	"check-circle":  {Body: `<circle cx="12" cy="12" r="9"/><path d="M8 12.5l3 3 5-6"/>`},
	"x-circle":      {Body: `<circle cx="12" cy="12" r="9"/><path d="M9 9l6 6M15 9l-6 6"/>`},
	"eye":           {Body: `<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z"/><circle cx="12" cy="12" r="3"/>`},
	"edit":          {Body: `<path d="M4 20h4L19 9l-4-4L4 16z"/><path d="M13.5 6.5l4 4"/>`},
	"trash":         {Body: `<path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3"/>`},
	"code":          {Body: `<path d="M8 7l-5 5 5 5M16 7l5 5-5 5M14 4l-4 16"/>`},
	"layers":        {Body: `<path d="M12 3l9 5-9 5-9-5z"/><path d="M3 13l9 5 9-5"/>`},
	"grid":          {Body: `<rect x="4" y="4" width="7" height="7" rx="1"/><rect x="13" y="4" width="7" height="7" rx="1"/><rect x="4" y="13" width="7" height="7" rx="1"/><rect x="13" y="13" width="7" height="7" rx="1"/>`},
	"rocket":        {Body: `<path d="M12 2c3 2 5 5.5 5 10l-2 4H9l-2-4c0-4.5 2-8 5-10z"/><circle cx="12" cy="10" r="2"/><path d="M9 16l-2 4M15 16l2 4M12 16v5"/>`},
	"target":        {Body: `<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1"/>`},
	"chart":         {Body: `<path d="M4 20V4M4 20h16M8 16v-4M12 16V8M16 16v-6"/>`},
	"trending":      {Body: `<path d="M3 17l6-6 4 4 8-8"/><path d="M15 7h6v6"/>`},
	"briefcase":     {Body: `<rect x="3" y="7" width="18" height="13" rx="2"/><path d="M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M3 13h18"/>`},
	"book":          {Body: `<path d="M4 5a2 2 0 0 1 2-2h14v16H6a2 2 0 0 0-2 2z"/><path d="M4 21V5M8 7h8"/>`},
	"palette":       {Body: `<path d="M12 3a9 9 0 0 0 0 18c1.5 0 2-1 2-2s-1-1.5-1-2.5 1-1.5 2-1.5h2a4 4 0 0 0 4-4c0-4.4-4-8-9-8z"/><circle cx="7.5" cy="11" r="1"/><circle cx="10.5" cy="7.5" r="1"/><circle cx="15" cy="8" r="1"/>`},
	"wifi":          {Body: `<path d="M2 9a15 15 0 0 1 20 0M5 12.5a10 10 0 0 1 14 0M8.5 16a5 5 0 0 1 7 0M12 19.5h.01"/>`},
	"quote":         {Body: `<path d="M10 7H6a2 2 0 0 0-2 2v3h5v2a4 4 0 0 1-4 4M20 7h-4a2 2 0 0 0-2 2v3h5v2a4 4 0 0 1-4 4"/>`},
	"sparkles":      {Body: `<path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8zM19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8z"/>`},
	"fire":          {Body: `<path d="M12 22a7 7 0 0 0 7-7c0-4-3-6-4-10-2 2-3 4-3 6-1-1-2-2-2-4-2 2-5 5-5 8a7 7 0 0 0 7 7z"/>`},
	"facebook":      {Body: `<path d="M14 8h3V4h-3a4 4 0 0 0-4 4v2H8v4h2v8h4v-8h3l1-4h-4V8.5c0-.3.2-.5.5-.5z"/>`, Fill: true},
	"instagram":     {Body: `<rect x="3" y="3" width="18" height="18" rx="5"/><circle cx="12" cy="12" r="4"/><path d="M17.5 6.5h.01"/>`},
	"youtube":       {Body: `<path fill-rule="evenodd" d="M21.6 7.2a2.6 2.6 0 0 0-1.8-1.8C18.2 5 12 5 12 5s-6.2 0-7.8.4A2.6 2.6 0 0 0 2.4 7.2 27 27 0 0 0 2 12a27 27 0 0 0 .4 4.8 2.6 2.6 0 0 0 1.8 1.8C5.8 19 12 19 12 19s6.2 0 7.8-.4a2.6 2.6 0 0 0 1.8-1.8A27 27 0 0 0 22 12a27 27 0 0 0-.4-4.8zM10 15V9l5.2 3z"/>`, Fill: true},
	"x-brand":       {Body: `<path d="M4 4h4.5L20 20h-4.5z"/><path d="M20 4l-6.6 7.3M4 20l6.6-7.3"/>`},
	"tiktok":        {Body: `<path d="M16 3c.3 2.3 1.9 4 4 4.2v3.3a7.5 7.5 0 0 1-4-1.3V15a6 6 0 1 1-6-6v3.4a2.6 2.6 0 1 0 2.6 2.6V3z"/>`, Fill: true},
	"line-chat":     {Body: `<path d="M12 3C6.5 3 2 6.6 2 11c0 3.9 3.5 7.2 8.3 7.9l-.4 2.3c-.1.5.4.8.8.5L16 18c3.6-1.3 6-3.9 6-7 0-4.4-4.5-8-10-8z"/>`, Fill: true},
	"linkedin":      {Body: `<path d="M4 9h4v11H4zM6 3.5a2 2 0 1 1 0 4 2 2 0 0 1 0-4zM10 9h3.8v1.6c.5-.9 1.8-1.9 3.7-1.9 4 0 4.5 2.5 4.5 5.8V20h-4v-4.8c0-1.2 0-2.7-1.7-2.7s-2 1.3-2 2.6V20h-4z"/>`, Fill: true},
}

// Shapes is the built-in decorative shape set.
var Shapes = map[string]Shape{
	// section dividers: 1200×120, stretched to the box
	"wave":     {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: `<path d="M0,64 C150,112 350,0 600,48 C850,96 1050,24 1200,56 L1200,120 L0,120 Z"/>`},
	"waves":    {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: `<path opacity=".3" d="M0,40 C200,100 400,0 600,40 C800,80 1000,10 1200,40 L1200,120 L0,120 Z"/><path opacity=".5" d="M0,64 C220,110 420,20 620,60 C820,100 1020,30 1200,66 L1200,120 L0,120 Z"/><path d="M0,86 C240,120 440,50 640,84 C840,118 1040,60 1200,90 L1200,120 L0,120 Z"/>`},
	"curve":    {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: `<path d="M0,120 Q600,-40 1200,120 Z"/>`},
	"curve-in": {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: `<path d="M0,0 Q600,160 1200,0 L1200,120 L0,120 Z"/>`},
	"tilt":     {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: `<path d="M0,120 L1200,0 L1200,120 Z"/>`},
	"triangle": {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: `<path d="M0,120 L600,10 L1200,120 Z"/>`},
	"arrow":    {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: `<path d="M0,120 L0,40 L560,40 L600,0 L640,40 L1200,40 L1200,120 Z"/>`},
	"zigzag":   {Kind: "divider", Stretch: true, ViewBox: "0 0 1200 120", Body: zigzag()},
	// shapes: 200×200, proportional
	"circle":  {Kind: "blob", ViewBox: "0 0 200 200", Body: `<circle cx="100" cy="100" r="96"/>`},
	"blob1":   {Kind: "blob", ViewBox: "0 0 200 200", Body: `<path d="M62,18 C104,-2 166,18 182,64 C198,110 172,128 176,158 C180,188 132,198 98,186 C64,174 28,190 14,150 C0,110 30,96 28,66 C26,40 40,28 62,18 Z"/>`},
	"blob2":   {Kind: "blob", ViewBox: "0 0 200 200", Body: `<path d="M118,12 C160,18 196,52 186,96 C178,132 146,134 138,166 C130,196 84,198 58,180 C30,162 2,138 12,96 C20,60 52,62 66,36 C78,16 96,8 118,12 Z"/>`},
	"blob3":   {Kind: "blob", ViewBox: "0 0 200 200", Body: `<path d="M100,20 C130,6 176,28 184,70 C192,112 170,132 168,160 C164,192 118,196 90,184 C60,172 18,168 14,124 C10,82 40,70 54,48 C66,30 78,30 100,20 Z"/>`},
	"hexagon": {Kind: "blob", ViewBox: "0 0 200 200", Body: `<path d="M100,8 L180,54 L180,146 L100,192 L20,146 L20,54 Z"/>`},
	"star":    {Kind: "blob", ViewBox: "0 0 200 200", Body: starPath()},
	"dots":    {Kind: "blob", ViewBox: "0 0 200 200", Body: dotGrid()},
}

func zigzag() string {
	var b strings.Builder
	b.WriteString(`<path d="M0,120 L0,70`)
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&b, " L%d,%d L%d,70", i*60+30, 100, i*60+60)
	}
	b.WriteString(` L1200,120 Z"/>`)
	return b.String()
}

func starPath() string {
	var pts []string
	for i := 0; i < 10; i++ {
		r := 96.0
		if i%2 == 1 {
			r = 42
		}
		a := float64(i)*math.Pi/5 - math.Pi/2
		pts = append(pts, fmt.Sprintf("%.1f,%.1f", 100+r*math.Cos(a), 104+r*math.Sin(a)))
	}
	return `<path d="M` + strings.Join(pts, " L") + ` Z"/>`
}

func dotGrid() string {
	var b strings.Builder
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="5"/>`, 16+x*24, 16+y*24)
		}
	}
	return b.String()
}

// IconSVG is the <svg> of an icon node (cls is the class list).
func IconSVG(n *Node, cls string) string {
	ic, ok := Icons[getStr(n.Props, "icon")]
	if !ok {
		ic = Icons["star"]
	}
	sw := 2.0
	if v, ok := getNum(n.Props, "strokeWidth"); ok && v > 0 && v <= 4 {
		sw = v
	}
	paint := `fill="none" stroke="currentColor" stroke-width="` + fnum(sw) + `" stroke-linecap="round" stroke-linejoin="round"`
	if ic.Fill {
		paint = `fill="currentColor" stroke="none"`
	}
	return `<svg class="` + cls + `" viewBox="0 0 24 24" ` + paint + a11y(n) + `>` + ic.Body + `</svg>`
}

// ButtonIcon is the icon of a button (props.icon, a name from Icons; 2.2.0):
// 1.15em, in the text colour, decorative (the text names the button). "" when
// the button has none or the name is unknown.
func ButtonIcon(n *Node) string {
	ic, ok := Icons[getStr(n.Props, "icon")]
	if !ok {
		return ""
	}
	paint := `fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"`
	if ic.Fill {
		paint = `fill="currentColor" stroke="none"`
	}
	return `<svg class="rea-btn-ic" viewBox="0 0 24 24" width="1.15em" height="1.15em" ` + paint + ` aria-hidden="true" focusable="false">` + ic.Body + `</svg>`
}

// ShapeSVG is the <svg> of a shape node.
func ShapeSVG(n *Node, cls string) string {
	sh, ok := Shapes[getStr(n.Props, "shape")]
	if !ok {
		sh = Shapes["wave"]
	}
	ratio := "xMidYMid meet"
	if sh.Stretch {
		ratio = "none"
	}
	return `<svg class="` + cls + `" viewBox="` + sh.ViewBox + `" preserveAspectRatio="` + ratio + `" fill="currentColor"` + a11y(n) + `>` + sh.Body + `</svg>`
}

// a11y: decorative unless the designer gave a label.
func a11y(n *Node) string {
	if l := strings.TrimSpace(getStr(n.Props, "label")); l != "" {
		return ` role="img" aria-label="` + attr(l) + `"`
	}
	return ` aria-hidden="true"`
}

// AssetCatalog is the icon and shape set for the designer (GET /design/assets).
func AssetCatalog() map[string]any {
	names := make([]string, 0, len(Icons))
	for k := range Icons {
		names = append(names, k)
	}
	sort.Strings(names)
	return map[string]any{"icons": Icons, "iconNames": names, "shapes": Shapes, "masks": Masks(), "maskNames": MaskNames()}
}

// BurgerSVG is the ☰ button of a menu that folds on phones.
const BurgerSVG = `<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 7h16M4 12h16M4 17h16"/></svg>`
