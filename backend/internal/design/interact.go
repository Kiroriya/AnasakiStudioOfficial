package design

import (
	"strconv"
	"strings"
)

// Interactions (Figma prototype → live site): a layer with props.action
// opens/closes a popup, shows/hides a layer (accordion), switches tabs or
// scrolls to a layer. props.popup turns a frame into a modal, props.collapsed
// hides a layer until an action shows it, and props.carousel turns a
// horizontal frame into a slider. The site gets InteractScript only on pages
// that use any of this.

// Action is one wired trigger: elements with class S do A to the layer with class T.
type Action struct {
	S string `json:"s"`
	A string `json:"a"`
	T string `json:"t,omitempty"`
}

var actionTypes = map[string]bool{"open": true, "close": true, "toggle": true, "tab": true, "scroll": true}

// actionOf reads props.action ({type, target}) of n.
func actionOf(n *Node) (Action, bool) {
	m, _ := n.Props["action"].(map[string]any)
	typ := getStr(m, "type")
	if !actionTypes[typ] {
		return Action{}, false
	}
	a := Action{S: ClassName(n.ID), A: typ}
	if t := getStr(m, "target"); t != "" {
		a.T = ClassName(t)
	} else if typ != "close" {
		return Action{}, false
	}
	return a, true
}

// prefixActions points actions inside a component placement at the
// placement's own copies of the target layers (ids "<instance>-<layer>").
func prefixActions(nodes []*Node, prefix, rootID string) {
	for _, c := range nodes {
		if m, ok := c.Props["action"].(map[string]any); ok {
			if t := getStr(m, "target"); t != "" {
				nt := prefix + "-" + t
				if t == rootID {
					nt = prefix
				}
				c.Props = cloneMap(c.Props)
				a := cloneMap(m)
				a["target"] = nt
				c.Props["action"] = a
			}
		}
		prefixActions(c.Children, prefix, rootID)
	}
}

// ActiveDecls is the look of a tab / accordion trigger while it is active
// (style.active: fill, color, stroke).
func ActiveDecls(n *Node) []Decl {
	a, _ := n.Style["active"].(map[string]any)
	var d []Decl
	if v := getStr(a, "fill"); v != "" {
		d = append(d, Decl{"background", safeCSS(v)})
	}
	if v := getStr(a, "color"); v != "" {
		d = append(d, Decl{"color", safeCSS(v)})
	}
	if v := getStr(a, "stroke"); v != "" {
		d = append(d, Decl{"border-color", safeCSS(v)})
	}
	return d
}

// popupOpen returns the wrapper that turns a frame into a modal.
func popupOpen(n *Node) string {
	delay := int(num(n.Props["popupDelay"]))
	if delay < 0 || delay > 600 {
		delay = 0
	}
	s := `<div class="rea-pop" role="dialog" aria-modal="true" data-id="` + ClassName(n.ID) + `"`
	if delay > 0 {
		s += ` data-delay="` + strconv.Itoa(delay) + `"`
		if getBool(n.Props, "popupOnce") {
			s += ` data-once="1"`
		}
	}
	return s + `><div class="rea-pop-bg"></div>`
}

// carouselClose ends a slider's track and adds its arrows and dots.
func carouselClose(n *Node) string {
	m, _ := n.Props["carousel"].(map[string]any)
	var b strings.Builder
	b.WriteString(`</div>`)
	if getBool(m, "arrows") {
		b.WriteString(`<button type="button" class="rea-car-btn rea-car-prev" aria-label="Previous">‹</button><button type="button" class="rea-car-btn rea-car-next" aria-label="Next">›</button>`)
	}
	if getBool(m, "dots") {
		b.WriteString(`<div class="rea-car-dots"></div>`)
	}
	return b.String()
}

func isCarousel(n *Node) bool {
	_, ok := n.Props["carousel"].(map[string]any)
	return ok && n.Type == TypeFrame
}

func carouselAutoplay(n *Node) int {
	m, _ := n.Props["carousel"].(map[string]any)
	if ap := int(num(m["autoplay"])); ap > 0 && ap <= 60 {
		return ap
	}
	return 0
}

// InteractCSS is added to pages that use interactions.
const InteractCSS = `.rea-collapsed:not(.rea-open){display:none!important}
.rea-pop{position:fixed;inset:0;z-index:1000;display:none;align-items:center;justify-content:center;padding:16px;overflow:auto}
.rea-pop.rea-open{display:flex}
.rea-pop.rea-open>:not(.rea-pop-bg){animation:rea-pop-in .22s ease-out}
.rea-pop-bg{position:fixed;inset:0;background:rgba(15,18,32,.55)}
.rea-pop>:not(.rea-pop-bg){position:relative!important;left:auto!important;top:auto!important;max-height:calc(100vh - 32px);overflow:auto;margin:auto}
@keyframes rea-pop-in{from{opacity:0;transform:translateY(10px) scale(.98)}}
html.rea-lock{overflow:hidden}
.rea-car{overflow:hidden}
.rea-track{display:flex;gap:inherit;align-items:inherit;overflow-x:auto;scroll-snap-type:x mandatory;scroll-behavior:smooth;scrollbar-width:none;flex:1 1 100%;min-width:0;width:100%}
.rea-track::-webkit-scrollbar{display:none}
.rea-track>*{flex-shrink:0!important;scroll-snap-align:start}
.rea-track>.rea-slide-fill{flex:0 0 100%!important;max-width:100%}
.rea-car-btn{position:absolute;top:50%;transform:translateY(-50%);z-index:2;width:42px;height:42px;border-radius:999px;border:0;background:rgba(255,255,255,.92);box-shadow:0 2px 10px rgba(0,0,0,.18);cursor:pointer;font-size:26px;line-height:1;color:#111827}
.rea-car-prev{left:12px}.rea-car-next{right:12px}
.rea-car-dots{position:absolute;left:0;right:0;bottom:14px;display:flex;justify-content:center;gap:6px;z-index:2}
.rea-car-dots button{width:8px;height:8px;border-radius:999px;border:0;padding:0;background:rgba(255,255,255,.6);box-shadow:0 0 0 1px rgba(0,0,0,.18);cursor:pointer;transition:width .2s}
.rea-car-dots button.on{background:#fff;width:22px}
`

// InteractScript wires the actions (window.__reaAct), popups and sliders.
const InteractScript = `(function(){var A=window.__reaAct||[],d=document,ea=function(l,f){[].forEach.call(l,f)};
function one(c){return d.querySelector('.'+c)}
function show(p){if(!p)return;p.classList.add('rea-open');d.documentElement.classList.add('rea-lock');var f=p.querySelector('a,button,input,textarea,select');f&&f.focus({preventScroll:true})}
function hide(p){if(!p)return;p.classList.remove('rea-open');if(!d.querySelector('.rea-pop.rea-open'))d.documentElement.classList.remove('rea-lock')}
function run(a,t){var el=a.t?one(a.t):null;
if(a.a==='open')show(el&&el.closest('.rea-pop'));
else if(a.a==='close')hide((el&&el.closest('.rea-pop'))||t.closest('.rea-pop'));
else if(a.a==='toggle'&&el){var c=el.classList,o;if(c.contains('rea-open')){c.remove('rea-open');o=false}else if(c.contains('rea-collapsed')){c.add('rea-open');o=true}else{c.add('rea-collapsed');o=false}t.classList.toggle('rea-active',o);t.setAttribute('aria-expanded',String(o))}
else if(a.a==='tab'&&el){ea(el.parentElement.children,function(s){var on=s===el;s.classList.toggle('rea-open',on);if(!on)s.classList.add('rea-collapsed')});ea(t.parentElement.children,function(s){s.classList.toggle('rea-active',s===t);if(s.getAttribute('role')==='tab')s.setAttribute('aria-selected',String(s===t))})}
else if(a.a==='scroll'&&el)el.scrollIntoView({behavior:'smooth',block:'start'})}
A.forEach(function(a){ea(d.querySelectorAll('.'+a.s),function(t){t.style.cursor='pointer';if(!/^(A|BUTTON)$/.test(t.tagName)){t.setAttribute('role','button');t.tabIndex=0}
t.addEventListener('click',function(e){e.preventDefault();run(a,t)});t.addEventListener('keydown',function(e){if(e.key==='Enter'||e.key===' '){e.preventDefault();run(a,t)}});
if(a.a==='tab'){var first=t===t.parentElement.firstElementChild,pn=a.t&&one(a.t);t.setAttribute('role','tab');t.setAttribute('aria-selected',String(first));t.parentElement.setAttribute('role','tablist');if(pn)pn.setAttribute('role','tabpanel');if(first)t.classList.add('rea-active')}})});
ea(d.querySelectorAll('.rea-pop'),function(p){p.querySelector('.rea-pop-bg').addEventListener('click',function(){hide(p)});var s=+p.dataset.delay;if(s>0)setTimeout(function(){var k='rea-pop-'+p.dataset.id;try{if(p.dataset.once){if(localStorage.getItem(k))return;localStorage.setItem(k,'1')}}catch(e){}show(p)},s*1000)});
d.addEventListener('keydown',function(e){if(e.key==='Escape')ea(d.querySelectorAll('.rea-pop.rea-open'),hide)});
ea(d.querySelectorAll('.rea-car'),function(c){var tr=c.querySelector('.rea-track');if(!tr)return;
function end(){return tr.scrollLeft+tr.clientWidth>=tr.scrollWidth-4}
function go(dir){if(dir>0&&end())tr.scrollTo({left:0,behavior:'smooth'});else if(dir<0&&tr.scrollLeft<4)tr.scrollTo({left:tr.scrollWidth,behavior:'smooth'});else tr.scrollBy({left:dir*tr.clientWidth,behavior:'smooth'})}
var pv=c.querySelector('.rea-car-prev'),nx=c.querySelector('.rea-car-next');pv&&pv.addEventListener('click',function(){go(-1)});nx&&nx.addEventListener('click',function(){go(1)});
var dots=c.querySelector('.rea-car-dots');if(dots){ea(tr.children,function(s,i){var b=d.createElement('button');b.type='button';b.setAttribute('aria-label',String(i+1));b.addEventListener('click',function(){tr.scrollTo({left:s.offsetLeft-tr.offsetLeft,behavior:'smooth'})});dots.appendChild(b)});
var mark=function(){var x=tr.scrollLeft,best=0,bd=1e9;ea(tr.children,function(s,i){var dd=Math.abs(s.offsetLeft-tr.offsetLeft-x);if(dd<bd){bd=dd;best=i}});ea(dots.children,function(b,i){b.classList.toggle('on',i===best)})};
tr.addEventListener('scroll',function(){clearTimeout(tr._t);tr._t=setTimeout(mark,80)});mark()}
var ap=+c.dataset.autoplay;if(ap>0&&!matchMedia('(prefers-reduced-motion: reduce)').matches){var hov=false;c.addEventListener('mouseenter',function(){hov=true});c.addEventListener('mouseleave',function(){hov=false});setInterval(function(){if(!hov&&!d.hidden)go(1)},ap*1000)}})})();`
