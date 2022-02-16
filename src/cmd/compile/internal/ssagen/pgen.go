// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssagen

import (
	"fmt"
	"internal/buildcfg"
	"internal/race"
	"math/rand"
	"sort"
	"sync"
	"time"

	"cmd/compile/internal/base"
	"cmd/compile/internal/ir"
	"cmd/compile/internal/objw"
	"cmd/compile/internal/ssa"
	"cmd/compile/internal/types"
	"cmd/internal/obj"
	"cmd/internal/objabi"
	"cmd/internal/src"
)

// cmpstackvarlt reports whether the stack variable a sorts before b.
//
// Sort the list of stack variables. Autos after anything else,
// within autos, unused after used, within used, things with
// pointers first, zeroed things first, and then decreasing size.
// Because autos are laid out in decreasing addresses
// on the stack, pointers first, zeroed things first and decreasing size
// really means, in memory, things with pointers needing zeroing at
// the top of the stack and increasing in size.
// Non-autos sort on offset.
func cmpstackvarlt(a, b *ir.Name) bool {
	if needAlloc(a) != needAlloc(b) {
		return needAlloc(b)
	}

	// 到这里needAlloc(a) == needAlloc(b)
	if !needAlloc(a) {
		return a.FrameOffset() < b.FrameOffset()
	}

	if a.Used() != b.Used() {
		return a.Used()
	}

	ap := a.Type().HasPointers()
	bp := b.Type().HasPointers()
	if ap != bp {
		return ap
	}

	ap = a.Needzero()
	bp = b.Needzero()
	if ap != bp {
		return ap
	}

	if a.Type().Width != b.Type().Width {
		return a.Type().Width > b.Type().Width
	}

	return a.Sym().Name < b.Sym().Name
}

// byStackvar implements sort.Interface for []*Node using cmpstackvarlt.
type byStackVar []*ir.Name

func (s byStackVar) Len() int           { return len(s) }
func (s byStackVar) Less(i, j int) bool { return cmpstackvarlt(s[i], s[j]) }
func (s byStackVar) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// needAlloc reports whether n is within the current frame, for which we need to
// allocate space. In particular, it excludes arguments and results, which are in
// the callers frame.
func needAlloc(n *ir.Name) bool {
	// 注意这里是在寄存器中出参才需要在本函数的栈上分配空间:
	//  - 出参在栈上，那么是在调用方的栈上，直接写入即可
	//  - 出参在寄存器上，那么需要记录在本函数栈上，返回时再写入寄存器，此时反而需要分配栈空间
	return n.Class == ir.PAUTO || n.Class == ir.PPARAMOUT && n.IsOutputParamInRegisters()
}

/*
// -gcflags="-N -l"
func f(x1, x2 int32) (x3, x4 int32) {
	var x5, x6 = 1, 100
	return x5, x6
}

ir.Name.FrameOffset():
	x2: 4
	x1: 0
	-------------frame base
	x3: -4
	x4: -8
	x5: -12
	x6: -16

	stksize=16

如果x1和x3之间需要存储其他数据，比如ret address, bp, lr等等，则x3-x6需要向下推，
即实际offset值变小，在 StackOffset 中计算

AMD64中，需要记录ret，bp, 所以x3-x6的StackOffset(n)返回值减去16, 而x1,x2返回值不变
*/

func (s *ssafn) AllocFrame(f *ssa.Func) {
	s.stksize = 0
	s.stkptrsize = 0
	fn := s.curfn

	xlog("==========AllocFrame: %v\n", fn)
	xlog("  len(Dcl): %v\n", len(fn.Dcl))
	xlog("  len(RegAlloc): %v\n", len(f.RegAlloc))
	xlog("  len(Blocks): %v\n", len(f.Blocks))

	// Mark the PAUTO's unused.
	for _, ln := range fn.Dcl {
		if needAlloc(ln) {
			ln.SetUsed(false)
		}
	}

	for _, l := range f.RegAlloc {
		if ls, ok := l.(ssa.LocalSlot); ok {
			ls.N.SetUsed(true)
		}
	}

	for _, b := range f.Blocks {
		for _, v := range b.Values {
			xlog("    block %v value %v; aux: %T, %v\n", b, v, v.Aux, v.Aux)
			if n, ok := v.Aux.(*ir.Name); ok {
				switch n.Class {
				case ir.PPARAMOUT:
					if n.IsOutputParamInRegisters() && v.Op == ssa.OpVarDef {
						// ignore VarDef, look for "real" uses.
						// TODO: maybe do this for PAUTO as well?
						continue
					}
					fallthrough
				case ir.PPARAM, ir.PAUTO:
					n.SetUsed(true)
				}
			}
		}
	}

	sort.Sort(byStackVar(fn.Dcl))

	// Reassign stack offsets of the locals that are used.
	lastHasPtr := false
	for i, n := range fn.Dcl {
		if n.Op() != ir.ONAME || n.Class != ir.PAUTO && !(n.Class == ir.PPARAMOUT && n.IsOutputParamInRegisters()) {
			// i.e., stack assign if AUTO, or if PARAMOUT in registers (which has no predefined spill locations)
			continue
		}
		if !n.Used() { // unused 一定会排在 used 后面，因此之后全部都是unused
			fn.Dcl = fn.Dcl[:i]
			break
		}

		types.CalcSize(n.Type())
		w := n.Type().Width
		if w >= types.MaxWidth || w < 0 {
			base.Fatalf("bad width")
		}
		if w == 0 && lastHasPtr {
			// Pad between a pointer-containing object and a zero-sized object.
			// This prevents a pointer to the zero-sized object from being interpreted
			// as a pointer to the pointer-containing object (and causing it
			// to be scanned when it shouldn't be). See issue 24993.
			w = 1
		}
		s.stksize += w
		s.stksize = types.Rnd(s.stksize, int64(n.Type().Align))
		if n.Type().HasPointers() {
			s.stkptrsize = s.stksize
			lastHasPtr = true
		} else {
			lastHasPtr = false
		}
		xlog("  SetFrameOffset: %v; %v\n", n, -s.stksize)
		n.SetFrameOffset(-s.stksize)
	}

	s.stksize = types.Rnd(s.stksize, int64(types.RegSize))
	s.stkptrsize = types.Rnd(s.stkptrsize, int64(types.RegSize))

	for _, ln := range fn.Dcl {
		xlog("  %v: offset: %v; width=%v; used=%v; stackoffset=%v\n",
			ln, ln.FrameOffset(), ln.Type().Width, ln.Used(), StackOffsetDebug(ln))
	}
}

const maxStackSize = 1 << 30

// Compile builds an SSA backend function,
// uses it to generate a plist,
// and flushes that plist to machine code.
// worker indicates which of the backend workers is doing the processing.
//
// 两个关键步骤: buildssa: ir.Node => []*ssa.Value
//               genssa: []*ssa.Value => []*prog.Prog
func Compile(fn *ir.Func, worker int) {
	f := buildssa(fn, worker)
	// Note: check arg size to fix issue 25507.
	if f.Frontend().(*ssafn).stksize >= maxStackSize || f.OwnAux.ArgWidth() >= maxStackSize {
		largeStackFramesMu.Lock()
		largeStackFrames = append(largeStackFrames, largeStack{locals: f.Frontend().(*ssafn).stksize, args: f.OwnAux.ArgWidth(), pos: fn.Pos()})
		largeStackFramesMu.Unlock()
		return
	}
	pp := objw.NewProgs(fn, worker)
	defer pp.Free()
	genssa(f, pp)
	// Check frame size again.
	// The check above included only the space needed for local variables.
	// After genssa, the space needed includes local variables and the callee arg region.
	// We must do this check prior to calling pp.Flush.
	// If there are any oversized stack frames,
	// the assembler may emit inscrutable complaints about invalid instructions.
	if pp.Text.To.Offset >= maxStackSize {
		largeStackFramesMu.Lock()
		locals := f.Frontend().(*ssafn).stksize
		largeStackFrames = append(largeStackFrames, largeStack{locals: locals, args: f.OwnAux.ArgWidth(), callee: pp.Text.To.Offset - locals, pos: fn.Pos()})
		largeStackFramesMu.Unlock()
		return
	}

	pp.Flush() // assemble, fill in boilerplate, etc.
	// fieldtrack must be called after pp.Flush. See issue 20014.
	fieldtrack(pp.Text.From.Sym, fn.FieldTrack)
}

func init() {
	if race.Enabled {
		rand.Seed(time.Now().UnixNano())
	}
}

func StackOffsetDebug(n *ir.Name) (v int32) {
	slot := ssa.LocalSlot{N: n}
	return StackOffset(slot)
}

// StackOffset returns the stack location of a LocalSlot relative to the
// stack pointer, suitable for use in a DWARF location entry. This has nothing
// to do with its offset in the user variable.
func StackOffset(slot ssa.LocalSlot) (v int32) {
	defer func() {
		xlog("StackOffset: %v -> %v\n", slot.N, v)
	}()
	n := slot.N
	var off int64
	switch n.Class {
	case ir.PPARAM, ir.PPARAMOUT:
		if !n.IsOutputParamInRegisters() {
			off = n.FrameOffset() + base.Ctxt.FixedFrameSize()
			break
		}
		fallthrough // PPARAMOUT in registers allocates like an AUTO
	case ir.PAUTO:
		off = n.FrameOffset()
		if base.Ctxt.FixedFrameSize() == 0 {
			off -= int64(types.PtrSize) // ret
		}
		if buildcfg.FramePointerEnabled {
			off -= int64(types.PtrSize) // save bp
		}
	}
	return int32(off + slot.Off)
}

// fieldtrack adds R_USEFIELD relocations to fnsym to record any
// struct fields that it used.
func fieldtrack(fnsym *obj.LSym, tracked map[*obj.LSym]struct{}) {
	if fnsym == nil {
		return
	}
	if !buildcfg.Experiment.FieldTrack || len(tracked) == 0 {
		return
	}

	trackSyms := make([]*obj.LSym, 0, len(tracked))
	for sym := range tracked {
		trackSyms = append(trackSyms, sym)
	}
	sort.Slice(trackSyms, func(i, j int) bool { return trackSyms[i].Name < trackSyms[j].Name })
	for _, sym := range trackSyms {
		r := obj.Addrel(fnsym)
		r.Sym = sym
		r.Type = objabi.R_USEFIELD
	}
}

// largeStack is info about a function whose stack frame is too large (rare).
type largeStack struct {
	locals int64
	args   int64
	callee int64
	pos    src.XPos
}

var (
	largeStackFramesMu sync.Mutex // protects largeStackFrames
	largeStackFrames   []largeStack
)

func CheckLargeStacks() {
	// Check whether any of the functions we have compiled have gigantic stack frames.
	sort.Slice(largeStackFrames, func(i, j int) bool {
		return largeStackFrames[i].pos.Before(largeStackFrames[j].pos)
	})
	for _, large := range largeStackFrames {
		if large.callee != 0 {
			base.ErrorfAt(large.pos, "stack frame too large (>1GB): %d MB locals + %d MB args + %d MB callee", large.locals>>20, large.args>>20, large.callee>>20)
		} else {
			base.ErrorfAt(large.pos, "stack frame too large (>1GB): %d MB locals + %d MB args", large.locals>>20, large.args>>20)
		}
	}
}

const debuglog = false

func xlog(format string, args ...interface{}) {
	if debuglog {
		fmt.Printf(format, args...)
	}
}
