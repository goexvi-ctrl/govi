import Cocoa

// GoviView is the editor surface: a custom NSView that draws one embedded
// engine's composed character grid and forwards key events into it. The engine
// runs in-process (via libgovi); this view is the "terminal" it renders to.
// Each window has its own GoviView bound to its own engine via `handle`.
final class GoviView: NSView, NSTextInputClient {
    // Engine-event modifier bits (must match engine.Mod).
    private static let modCtrl: Int32 = 1
    private static let modAlt: Int32 = 2

    // Special-key codes (must match engine.SpecialKey).
    private enum SK {
        static let escape: Int32 = 1
        static let enter: Int32 = 2
        static let backspace: Int32 = 4
        static let delete: Int32 = 5
        static let up: Int32 = 6
        static let down: Int32 = 7
        static let left: Int32 = 8
        static let right: Int32 = 9
        static let home: Int32 = 10
        static let end: Int32 = 11
        static let pageUp: Int32 = 12
        static let pageDown: Int32 = 13
    }

    // The handle of this view's embedded engine (one per window).
    let handle: Int64

    // documentTitle is the file name shown in the window title bar.
    var documentTitle = "Untitled"

    // Pending marked (uncommitted) text from a dead key or IME, e.g. the "¨"
    // after Option-u, shown at the cursor until the next key composes it.
    private var markedText = ""

    private var font = Settings.editorFont
    private var cellW: CGFloat = 8
    private var cellH: CGFloat = 16

    // Per-tab color specs, synced from the engine (:set / .exrc). Empty = system default.
    private var foregroundColorSpec = ""
    private var backgroundColorSpec = ""

    private var bgColor = NSColor.textBackgroundColor
    private var fgColor = NSColor.textColor
    private let cursorColor = NSColor.systemBlue

    // Inset (pixels) between the window edge and the text grid, from Settings.
    private var padding: CGFloat = Settings.padding

    private var rows = 1
    private var cols = 1
    private var timer: Timer?

    // Selection: buffer caret range by default; Option+mouse uses a screen
    // rectangle; overlay/ex without Option uses reading-order screen cells.
    private typealias Caret = (line: Int64, col: Int)
    private typealias ScreenCell = (x: Int32, y: Int32)
    private enum SelStyle { case buffer, linearScreen, rectangular }
    private var selActive = false
    private var selStyle: SelStyle = .buffer
    private var bufSelStart: Caret = (1, 0)
    private var bufSelEnd: Caret = (1, 0)
    private var bufDragAnchor: Caret = (1, 0)
    private var screenSelStart: ScreenCell = (0, 0)
    private var screenSelEnd: ScreenCell = (0, 0)
    private var screenDragAnchor: ScreenCell = (0, 0)
    private var dragging = false

    // shiftKeyDown tracks the physical shift key via flagsChanged. mouseDown's
    // modifierFlags often omits shift even when the key is held (especially on
    // the second click in a shift-click sequence within the double-click window).
    private var shiftKeyDown = false
    // True while shift is physically held (latched on press, cleared on release).
    // mouseDown often omits shift from modifierFlags even when this is true.
    private var shiftKeyEngaged = false

    // selGranularity records how the selection was started; shift-extend and
    // drag-extend snap to word or line boundaries after double/triple-click.
    private enum SelGranularity { case character, word, line }
    private var selGranularity: SelGranularity = .character

    // Spell checking (continuous): red squiggles under misspelled words on the
    // visible lines. Results are cached by line text so unchanged lines are not
    // re-checked. Ranges are rune (Unicode scalar) indices, matching engine cols.
    private typealias RuneRange = (start: Int, end: Int)
    private struct Misspelling { let line: Int64; let start: Int; let end: Int }
    private var spellEnabled = Settings.spellChecking
    private let spellTag = NSSpellChecker.uniqueSpellDocumentTag()
    private var spellCache: [String: [RuneRange]] = [:]
    private var misspellings: [Misspelling] = []
    private var contextMisspelling: Misspelling?

    init(frame frameRect: NSRect, handle: Int64) {
        self.handle = handle
        super.init(frame: frameRect)
        measureFont()
        observeSettings()
    }
    required init?(coder: NSCoder) {
        self.handle = 0
        super.init(coder: coder)
        measureFont()
        observeSettings()
    }

    deinit {
        NotificationCenter.default.removeObserver(self)
    }

    private func observeSettings() {
        NotificationCenter.default.addObserver(
            self, selector: #selector(settingsChanged), name: Settings.changed, object: nil)
    }

    private func applyColors() {
        fgColor = ColorParser.parse(foregroundColorSpec) ?? NSColor.textColor
        bgColor = ColorParser.parse(backgroundColorSpec) ?? NSColor.textBackgroundColor
    }

    private func syncColorsFromEngine() {
        var changed = false
        if let c = GoviForegroundSpec(handle) {
            let spec = String(cString: c)
            GoviFree(c)
            if spec != foregroundColorSpec {
                foregroundColorSpec = spec
                changed = true
            }
        }
        if let c = GoviBackgroundSpec(handle) {
            let spec = String(cString: c)
            GoviFree(c)
            if spec != backgroundColorSpec {
                backgroundColorSpec = spec
                changed = true
            }
        }
        if changed {
            applyColors()
        }
    }

    @objc private func settingsChanged() {
        padding = Settings.padding
        spellEnabled = Settings.spellChecking
        font = Settings.editorFont
        measureFont()
        resizeWindowToCells() // keep the same rows x cols; grow/shrink the window to fit
        updateGeometry()      // refits only if the window could not take the target size
        updateSpelling()
        updateTitle()
        needsDisplay = true
    }

    // textRows is the editable line count (the status line is excluded).
    var textRows: Int { max(1, rows - 1) }

    // contentSize returns the view size needed for textRows x cols of editor text
    // plus one status line, using the given font metrics and padding.
    static func contentSize(
        textRows: Int, cols: Int,
        font: NSFont = Settings.editorFont, padding: CGFloat = Settings.padding
    ) -> NSSize {
        let attrs: [NSAttributedString.Key: Any] = [.font: font]
        let cellW = ("0" as NSString).size(withAttributes: attrs).width
        let lm = NSLayoutManager()
        let cellH = lm.defaultLineHeight(for: font)
        let screenRows = textRows + 1
        // Round up: AppKit rounds the content rect to whole points, and rounding
        // down would shave a partial cell and lose a row/column.
        return NSSize(
            width: ceil(padding * 2 + CGFloat(cols) * cellW),
            height: ceil(padding * 2 + CGFloat(screenRows) * cellH)
        )
    }

    // cellPoint / cellRect convert a (col, row) cell to view pixels, applying the
    // padding inset; cellOf is the inverse for hit-testing.
    private func cellPoint(_ col: Int, _ row: Int) -> NSPoint {
        NSPoint(x: padding + CGFloat(col) * cellW, y: padding + CGFloat(row) * cellH)
    }

    private func cellRect(_ col: Int, _ row: Int) -> NSRect {
        NSRect(origin: cellPoint(col, row), size: NSSize(width: cellW, height: cellH))
    }

    private func measureFont() {
        let attrs: [NSAttributedString.Key: Any] = [.font: font]
        cellW = ("0" as NSString).size(withAttributes: attrs).width
        let lm = NSLayoutManager()
        cellH = lm.defaultLineHeight(for: font)
    }

    // resizeWindowToCells sizes the window to keep the current rows x cols of
    // text after a font or padding change, instead of re-fitting the grid into
    // the old pixel size. The top-left corner stays fixed. No-op without a window
    // (updateGeometry then handles the fallback).
    private func resizeWindowToCells() {
        guard let window = window else { return }
        let content = GoviView.contentSize(textRows: textRows, cols: cols, font: font, padding: padding)
        let newFrameSize = window.frameRect(forContentRect: NSRect(origin: .zero, size: content)).size
        var frame = window.frame
        frame.origin.y += frame.size.height - newFrameSize.height // keep the top edge put
        frame.size = newFrameSize
        window.setFrame(frame, display: true)
    }

    // The grid's first row is at the top, so use a flipped coordinate system.
    override var isFlipped: Bool { true }
    override var acceptsFirstResponder: Bool { true }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        window?.makeFirstResponder(self)
        syncShiftKeyState()
        updateGeometry()
    }

    override func flagsChanged(with event: NSEvent) {
        syncShiftKeyState()
        let mask = NSEvent.ModifierFlags.deviceIndependentFlagsMask
        if !shiftPhysicallyDown()
            && !event.modifierFlags.intersection(mask).contains(.shift) {
            shiftKeyEngaged = false
        }
    }

    private func syncShiftKeyState() {
        let mask = NSEvent.ModifierFlags.deviceIndependentFlagsMask
        let physical = shiftPhysicallyDown()
        shiftKeyDown = NSEvent.modifierFlags.intersection(mask).contains(.shift) || physical
        if physical || shiftKeyDown {
            shiftKeyEngaged = true
        }
    }

    private func shiftPhysicallyDown() -> Bool {
        CGEventSource.flagsState(.hidSystemState).contains(.maskShift)
    }

    private func shiftHeld(_ event: NSEvent) -> Bool {
        if shiftKeyDown || shiftPhysicallyDown() { return true }
        let mask = NSEvent.ModifierFlags.deviceIndependentFlagsMask
        return event.modifierFlags.intersection(mask).contains(.shift)
            || NSEvent.modifierFlags.intersection(mask).contains(.shift)
    }

    private func shouldShiftExtend(_ event: NSEvent) -> Bool {
        shiftHeld(event) || (selActive && shiftKeyEngaged)
    }

    override func setFrameSize(_ newSize: NSSize) {
        super.setFrameSize(newSize)
        updateGeometry()
    }

    // updateGeometry recomputes the cell rows/cols for the current bounds and
    // tells the engine, then recomposes and repaints.
    func updateGeometry() {
        // Add a small epsilon so an exact fit isn't lost to sub-pixel rounding
        // (e.g. 80*cellW coming back as 79.9999.../cellW).
        let w = max(1, Int((bounds.width - 2 * padding) / cellW + 1e-6))
        let h = max(1, Int((bounds.height - 2 * padding) / cellH + 1e-6))
        if w == cols && h == rows { return }
        cols = w
        rows = h
        GoviResize(handle, Int32(rows), Int32(cols))
        recompose()
        updateTitle()
        needsDisplay = true
    }

    // MARK: - Engine step

    // step is run after every input: it handles quit, bell, title, recomposes
    // the grid, repaints, and arms any pending timer (map/showmatch/recovery).
    func step() {
        if window?.isKeyWindow == true {
            syncWorkingDirectory()
        }
        if GoviShouldQuit(handle) != 0 {
            // windowShouldClose prompts when there are unsaved changes.
            window?.close()
            return
        }
        if GoviTakeBell(handle) != 0 {
            NSSound.beep()
        }
        updateTitle()
        recompose()
        needsDisplay = true
        armTimer()
    }

    private func recompose() {
        GoviCompose(handle, Int32(rows), Int32(cols))
        syncColorsFromEngine()
        updateSpelling()
    }

    // syncWorkingDirectory sets the process cwd to this tab's directory so
    // :r/:w relative paths and shell commands match the focused editor.
    func syncWorkingDirectory() {
        guard let c = GoviCwd(handle) else { return }
        let dir = String(cString: c)
        GoviFree(c)
        if !dir.isEmpty {
            FileManager.default.changeCurrentDirectoryPath(dir)
        }
    }

    func updateTitle() {
        var base = documentTitle
        if let c = GoviTitle(handle) {
            let t = String(cString: c)
            GoviFree(c)
            if !t.isEmpty {
                base = t
            }
        }
        guard let w = window else { return }
        // Tab labels use the document name only; dimensions go in the window
        // subtitle (title bar), shared across all tabs in the group.
        w.title = base
        if Settings.showDimensionsInTitle && (w.isKeyWindow || w.tabGroup == nil) {
            w.subtitle = "\(textRows)x\(cols)"
        } else {
            w.subtitle = ""
        }
    }

    private func armTimer() {
        timer?.invalidate()
        timer = nil
        var interval: TimeInterval = 0
        var action: (() -> Void)?
        if GoviMatchPending(handle) != 0 {
            interval = Double(GoviMatchTimeMS(handle)) / 1000.0
            action = { GoviFireTimeout(self.handle) }
        } else if GoviMapPending(handle) != 0 {
            interval = 0.5
            action = { GoviFireTimeout(self.handle) }
        } else if GoviNeedsRecoverySync(handle) != 0 {
            interval = 2.0
            action = { GoviSyncRecovery(self.handle) }
        }
        guard let act = action else { return }
        timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: false) { [weak self] _ in
            act()
            self?.step()
        }
    }

    // MARK: - Mouse selection

    private func cellAt(_ event: NSEvent) -> (x: Int32, y: Int32) {
        let p = convert(event.locationInWindow, from: nil)
        let x = max(0, Int((p.x - padding) / cellW))
        let y = max(0, Int((p.y - padding) / cellH))
        return (Int32(x), Int32(y))
    }

    private func optionRectSelect(_ event: NSEvent) -> Bool {
        event.modifierFlags.intersection(.deviceIndependentFlagsMask).contains(.option)
    }

    private func selectionStyle(for event: NSEvent) -> SelStyle {
        if optionRectSelect(event) { return .rectangular }
        if GoviOverlayActive(handle) != 0 || GoviExActive(handle) != 0 { return .linearScreen }
        // In the editor, a click that lands off buffer text -- the status/command
        // row, ~ filler, or the line-number gutter -- selects displayed screen
        // text (copy-only). Otherwise GoviCellToPos clamps to the buffer line
        // above and the wrong text is selected.
        let c = cellAt(event)
        var line: Int64 = 0, col: Int32 = 0
        if GoviScreenToBuffer(handle, c.x, c.y, &line, &col) == 0 {
            return .linearScreen
        }
        return .buffer
    }

    private func clearSelection() {
        if !selActive { return }
        selActive = false
        GoviSetSelection(handle, 0, 0, 0, 0, 0)
        GoviSetScreenSelection(handle, 0, 0, 0, 0, 0, 0)
    }

    private func setBufferSelection(_ a: Caret, _ b: Caret) {
        if a.line == b.line && a.col == b.col {
            clearSelection()
            return
        }
        selActive = true
        selStyle = .buffer
        bufSelStart = a
        bufSelEnd = b
        GoviSetSelection(handle, 1, a.line, Int32(a.col), b.line, Int32(b.col))
    }

    private func setScreenSelection(_ style: SelStyle, _ a: ScreenCell, _ b: ScreenCell) {
        if a.x == b.x && a.y == b.y {
            clearSelection()
            return
        }
        selActive = true
        selStyle = style
        screenSelStart = a
        screenSelEnd = b
        let linear: Int32 = style == .linearScreen ? 1 : 0
        GoviSetScreenSelection(handle, 1, linear, a.x, a.y, b.x, b.y)
    }

    private func caretAt(_ event: NSEvent) -> Caret {
        let c = cellAt(event)
        var line: Int64 = 0
        var col: Int32 = 0
        GoviCellToPos(handle, c.x, c.y, &line, &col)
        return (line, Int(col))
    }

    private func cursorCaret() -> Caret {
        var line: Int64 = 0
        var col: Int32 = 0
        GoviCellToPos(handle, GoviCursorX(handle), GoviCursorY(handle), &line, &col)
        return (line, Int(col))
    }

    private func caretBefore(_ a: Caret, _ b: Caret) -> Bool {
        a.line < b.line || (a.line == b.line && a.col < b.col)
    }

    private func cellBefore(_ a: ScreenCell, _ b: ScreenCell) -> Bool {
        a.y < b.y || (a.y == b.y && a.x < b.x)
    }

    // Buffer caret range for cut/paste/replace/delete. Only linear buffer and
    // linear screen selections map to a single editable range; rectangular
    // (Option+drag) and linear screen selections are copy-only.
    private func bufferRangeForSelection() -> (Int64, Int, Int64, Int)? {
        guard selActive else { return nil }
        switch selStyle {
        case .buffer:
            let a = bufSelStart, b = bufSelEnd
            if caretBefore(b, a) {
                return (b.line, b.col, a.line, a.col)
            }
            return (a.line, a.col, b.line, b.col)
        case .rectangular, .linearScreen:
            return nil
        }
    }

    private func editBeep() {
        NSSound.beep()
    }

    private func moveCursorToCaret(_ caret: Caret) {
        GoviMoveCursor(handle, caret.line, Int32(caret.col))
    }

    private func screenWordRange(at cell: ScreenCell) -> (ScreenCell, ScreenCell) {
        guard let row = screenRowText(Int(cell.y)) else { return (cell, cell) }
        let (start, end) = wordBounds(in: row, at: Int(cell.x))
        return ((Int32(start), cell.y), (Int32(end), cell.y))
    }

    private func screenLineRange(at cell: ScreenCell) -> (ScreenCell, ScreenCell) {
        let len = screenRowText(Int(cell.y))?.count ?? 0
        return ((0, cell.y), (Int32(len), cell.y))
    }

    private enum ClickClass { case blank, word, punct }

    private func clickClass(_ ch: Character) -> ClickClass {
        if ch == " " || ch == "\t" { return .blank }
        if ch.isLetter || ch.isNumber || ch == "_" { return .word }
        return .punct
    }

    private func wordBounds(in row: String, at col: Int) -> (Int, Int) {
        let chars = Array(row)
        guard !chars.isEmpty else { return (0, 0) }
        var c = col
        if c >= chars.count { c = chars.count - 1 }
        if c < 0 { c = 0 }
        let cls = clickClass(chars[c])
        var start = c, end = c + 1
        while start > 0 && clickClass(chars[start - 1]) == cls { start -= 1 }
        while end < chars.count && clickClass(chars[end]) == cls { end += 1 }
        return (start, end)
    }

    private func orderedBufSelEnds() -> (lo: Caret, hi: Caret) {
        if caretBefore(bufSelEnd, bufSelStart) {
            return (bufSelEnd, bufSelStart)
        }
        return (bufSelStart, bufSelEnd)
    }

    private func orderedScreenSelEnds() -> (lo: ScreenCell, hi: ScreenCell) {
        if cellBefore(screenSelEnd, screenSelStart) {
            return (screenSelEnd, screenSelStart)
        }
        return (screenSelStart, screenSelEnd)
    }

    private func extendBufferEndpoint(at event: NSEvent, fixedAnchor: Caret, extending: Bool) -> Caret {
        let click = caretAt(event)
        let c = cellAt(event)
        switch selGranularity {
        case .character:
            return click
        case .word:
            var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
            GoviWordRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
            let wStart: Caret = (l1, Int(c1))
            let wEnd: Caret = (l2, Int(c2))
            if extending {
                let bounds = orderedBufSelEnds()
                if caretBefore(click, bounds.lo) { return wStart }
                if caretBefore(bounds.hi, click) { return wEnd }
            }
            return caretBefore(wEnd, fixedAnchor) ? wStart : wEnd
        case .line:
            var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
            GoviLineRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
            let lStart: Caret = (l1, Int(c1))
            let lEnd: Caret = (l2, Int(c2))
            if extending {
                let bounds = orderedBufSelEnds()
                if caretBefore(click, bounds.lo) { return lStart }
                if caretBefore(bounds.hi, click) { return lEnd }
            }
            return caretBefore(lEnd, fixedAnchor) ? lStart : lEnd
        }
    }

    private func extendScreenEndpoint(at event: NSEvent, fixedAnchor: ScreenCell, extending: Bool) -> ScreenCell {
        let c = cellAt(event)
        switch selGranularity {
        case .character:
            return (c.x, c.y)
        case .word:
            let (wStart, wEnd) = screenWordRange(at: (c.x, c.y))
            if extending {
                let bounds = orderedScreenSelEnds()
                if cellBefore((c.x, c.y), bounds.lo) { return wStart }
                if cellBefore(bounds.hi, (c.x, c.y)) { return wEnd }
            }
            return cellBefore(wEnd, fixedAnchor) ? wStart : wEnd
        case .line:
            let (lStart, lEnd) = screenLineRange(at: (c.x, c.y))
            if extending {
                let bounds = orderedScreenSelEnds()
                if cellBefore((c.x, c.y), bounds.lo) { return lStart }
                if cellBefore(bounds.hi, (c.x, c.y)) { return lEnd }
            }
            return cellBefore(lEnd, fixedAnchor) ? lStart : lEnd
        }
    }

    private func extendSelection(to event: NSEvent) {
        let extending = selActive
        switch selStyle {
        case .buffer:
            let anchor = bufDragAnchor
            let snapped = extendBufferEndpoint(at: event, fixedAnchor: anchor, extending: extending)
            if extending {
                let bounds = orderedBufSelEnds()
                let click = caretAt(event)
                if caretBefore(click, anchor) {
                    setBufferSelection(snapped, bounds.hi)
                    moveCursorToCaret(snapped)
                } else {
                    setBufferSelection(bounds.lo, snapped)
                    moveCursorToCaret(snapped)
                }
            } else {
                setBufferSelection(anchor, snapped)
                moveCursorToCaret(snapped)
            }
        case .linearScreen, .rectangular:
            // Screen selections are copy-only; never move the cursor, even when
            // the drag extends over buffer cells.
            let anchor = screenDragAnchor
            let snapped = extendScreenEndpoint(at: event, fixedAnchor: anchor, extending: extending)
            if extending {
                let bounds = orderedScreenSelEnds()
                let cell = cellAt(event)
                if cellBefore((cell.x, cell.y), anchor) {
                    setScreenSelection(selStyle, snapped, bounds.hi)
                } else {
                    setScreenSelection(selStyle, bounds.lo, snapped)
                }
            } else {
                setScreenSelection(selStyle, anchor, snapped)
            }
        }
        step()
    }

    private func handleShiftExtendMouseDown(_ event: NSEvent, style: SelStyle) {
        if !selActive {
            selStyle = style
            if style == .buffer {
                bufDragAnchor = cursorCaret()
            } else {
                screenDragAnchor = (GoviCursorX(handle), GoviCursorY(handle))
            }
            switch event.clickCount {
            case 2: selGranularity = .word
            case 3: selGranularity = .line
            default: selGranularity = .character
            }
        }
        dragging = true
        extendSelection(to: event)
    }

    override func mouseDown(with event: NSEvent) {
        window?.makeFirstResponder(self)
        syncShiftKeyState()
        let style = selectionStyle(for: event)

        // Shift-click (or shift-click-drag) extends the selection. This must run
        // before the clickCount==2/3 handler: within the system double-click
        // interval AppKit often delivers clickCount=2 on the shift-click even at
        // a different location, which would otherwise replace the selection.
        if shouldShiftExtend(event) {
            handleShiftExtendMouseDown(event, style: style)
            return
        }

        if event.clickCount == 2 || event.clickCount == 3 {
            let c = cellAt(event)
            dragging = true
            if style == .buffer {
                var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
                if event.clickCount == 2 {
                    GoviWordRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
                    bufDragAnchor = (l1, Int(c1))
                    selGranularity = .word
                } else {
                    GoviLineRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
                    bufDragAnchor = (l1, Int(c1))
                    selGranularity = .line
                }
                setBufferSelection((l1, Int(c1)), (l2, Int(c2)))
                moveCursorToCaret((l2, Int(c2)))
            } else {
                let start: ScreenCell
                let end: ScreenCell
                if event.clickCount == 2 {
                    (start, end) = screenWordRange(at: (c.x, c.y))
                    screenDragAnchor = start
                    selGranularity = .word
                } else {
                    (start, end) = screenLineRange(at: (c.x, c.y))
                    screenDragAnchor = start
                    selGranularity = .line
                }
                setScreenSelection(style, start, end)
            }
            step()
            return
        }

        let cell = cellAt(event)
        dragging = true
        selGranularity = .character
        clearSelection()
        selStyle = style
        if style == .buffer {
            let caret = caretAt(event)
            bufDragAnchor = caret
            moveCursorToCaret(caret)
        } else {
            screenDragAnchor = (cell.x, cell.y)
        }
        step()
    }

    override func mouseDragged(with event: NSEvent) {
        guard dragging else { return }
        extendSelection(to: event)
    }

    override func mouseUp(with event: NSEvent) {
        dragging = false
    }

    // Wheel / trackpad scrolling moves the viewport like any windowed app; the
    // cursor stays put (it may scroll off-screen). Fractional trackpad deltas
    // accumulate so scrolling is smooth.
    private var scrollAccum: CGFloat = 0

    override func scrollWheel(with event: NSEvent) {
        if event.hasPreciseScrollingDeltas {
            scrollAccum += event.scrollingDeltaY / cellH // points -> lines
        } else {
            scrollAccum += event.scrollingDeltaY // already in lines
        }
        let lines = Int(scrollAccum.rounded(.towardZero))
        guard lines != 0 else { return }
        scrollAccum -= CGFloat(lines)
        // Positive scrollingDeltaY reveals earlier lines (top decreases).
        GoviScroll(handle, Int32(-lines))
        step()
    }

    // The tab bar's "+" button is shown by AppKit when this is found in the key
    // window's responder chain; it adds a new tab to this window's group.
    @objc override func newWindowForTab(_ sender: Any?) {
        EditorWindow.openTab(in: window, path: "")
    }

    // MARK: - Clipboard (Edit menu / standard shortcuts)

    @objc func copy(_ sender: Any?) {
        guard selActive else { return }
        let s: String
        switch selStyle {
        case .buffer:
            let a = bufSelStart, b = bufSelEnd
            s = bridgeString(GoviRangeText(handle, a.line, Int32(a.col), b.line, Int32(b.col)))
        case .rectangular:
            s = bridgeString(GoviScreenRangeText(handle, screenSelStart.x, screenSelStart.y,
                                                 screenSelEnd.x, screenSelEnd.y))
        case .linearScreen:
            s = bridgeString(GoviScreenLinearRangeText(handle, screenSelStart.x, screenSelStart.y,
                                                       screenSelEnd.x, screenSelEnd.y))
        }
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(s, forType: .string)
    }

    @objc func cut(_ sender: Any?) {
        guard selActive else { return }
        guard let r = bufferRangeForSelection() else { editBeep(); return }
        copy(sender)
        GoviDeleteRange(handle, r.0, Int32(r.1), r.2, Int32(r.3))
        clearSelection()
        step()
    }

    @objc func paste(_ sender: Any?) {
        guard let s = NSPasteboard.general.string(forType: .string) else { return }
        var buf = Array(s.utf8CString)
        if selActive {
            guard let r = bufferRangeForSelection() else { editBeep(); return }
            buf.withUnsafeMutableBufferPointer {
                GoviReplaceText(handle, r.0, Int32(r.1), r.2, Int32(r.3), $0.baseAddress)
            }
            clearSelection()
        } else {
            buf.withUnsafeMutableBufferPointer { GoviInsertText(handle, $0.baseAddress) }
        }
        step()
    }

    @objc override func selectAll(_ sender: Any?) {
        if GoviOverlayActive(handle) != 0 || GoviExActive(handle) != 0 {
            let n = Int(GoviRows(handle))
            let c = Int(GoviCols(handle))
            setScreenSelection(.linearScreen, (0, 0), (Int32(max(0, c - 1)), Int32(max(0, n - 1))))
        } else {
            var line: Int64 = 0, col: Int32 = 0
            GoviEndPos(handle, &line, &col)
            setBufferSelection((1, 0), (line, Int(col)))
        }
        step()
    }

    private func bridgeString(_ c: UnsafeMutablePointer<CChar>?) -> String {
        guard let c = c else { return "" }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    // MARK: - Input

    // keyDown splits input two ways. Control-modified keys are vi commands and
    // are translated directly, so Cocoa's built-in Emacs-style key bindings
    // (which would turn ^F/^B/^A/... into cursor motions) never see them.
    // Everything else is handed to the text input system via interpretKeyEvents
    // so plain typing, Option-accents (Option-o -> o-slash), dead keys
    // (Option-u then u -> u-umlaut), and IMEs compose correctly; the composed
    // result arrives back through the NSTextInputClient methods below.
    // Autorepeat (isARepeat) is handled directly: interpretKeyEvents does not
    // deliver repeats to insertText/doCommand.
    override func keyDown(with event: NSEvent) {
        if isControlKey(event) {
            handleControlKey(event)
            return
        }
        if event.isARepeat {
            handleRepeatedKey(event)
            return
        }
        interpretKeyEvents([event])
    }

    // handleRepeatedKey feeds held-key repeats straight to the engine.
    private func handleRepeatedKey(_ event: NSEvent) {
        guard markedText.isEmpty else { return }
        if event.keyCode == 48 {
            dispatchTab()
            return
        }
        if let key = specialKey(for: event) {
            dispatchSpecialKey(key)
            return
        }
        guard let chars = event.characters, !chars.isEmpty else { return }
        if replaceWithText(chars) { return }
        for scalar in chars.unicodeScalars {
            GoviKeyRune(handle, Int32(scalar.value), 0)
        }
        step()
    }

    // specialKey maps an NSEvent key code to an engine special key, if any.
    private func specialKey(for event: NSEvent) -> Int32? {
        switch event.keyCode {
        case 36: return SK.enter
        case 51: return SK.backspace
        case 117: return SK.delete
        case 53: return SK.escape
        case 123: return SK.left
        case 124: return SK.right
        case 125: return SK.down
        case 126: return SK.up
        case 115: return SK.home
        case 119: return SK.end
        case 116: return SK.pageUp
        case 121: return SK.pageDown
        default: return nil
        }
    }

    // dispatchSpecialKey handles a non-text key the same way doCommand would.
    private func dispatchSpecialKey(_ key: Int32) {
        switch key {
        case SK.enter:
            if replaceWithText("\n") { return }
            sendSpecial(SK.enter)
        case SK.backspace, SK.delete:
            if selActive { deleteSelection(); return }
            sendSpecial(key)
        case SK.escape:
            if selActive { clearSelection() }
            sendSpecial(SK.escape)
        default:
            sendSpecial(key)
        }
    }

    // isControlKey reports whether event is a control-modified keystroke. Use the
    // device-independent modifier mask (not raw modifierFlags) and accept C0 bytes
    // in `characters` — macOS often delivers ^A as SOH without .control set.
    private func isControlKey(_ event: NSEvent) -> Bool {
        let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
        if flags.contains(.control) { return true }
        if let c = event.characters?.unicodeScalars.first, c.value >= 1 && c.value <= 31 {
            return true
        }
        return false
    }

    // handleControlKey feeds vi/ex control bytes to the engine. Colon-line editing
    // expects the C0 code (^A -> SOH), not a letter with a modifier flag.
    private func handleControlKey(_ event: NSEvent) {
        if selActive { clearSelection() } // a command cancels the selection
        var mods: Int32 = 0
        if event.modifierFlags.intersection(.deviceIndependentFlagsMask).contains(.option) {
            mods |= GoviView.modAlt
        }
        let scalars: [Unicode.Scalar]
        if let c = event.characters, let first = c.unicodeScalars.first, first.value >= 1 && first.value <= 31 {
            scalars = Array(c.unicodeScalars)
        } else if let raw = event.charactersIgnoringModifiers {
            scalars = Array(raw.unicodeScalars)
        } else {
            return
        }
        for scalar in scalars {
            let code: Int32
            if scalar.value <= 31 {
                code = Int32(scalar.value)
            } else {
                code = Int32(scalar.value & 0x1f)
            }
            GoviKeyRune(handle, code, mods)
        }
        step()
    }

    // replaceWithText replaces the active selection with s and returns true, or
    // returns false if there was no selection (GUI replace-on-type / paste).
    private func replaceWithText(_ s: String) -> Bool {
        guard selActive else { return false }
        guard let r = bufferRangeForSelection() else { editBeep(); return true }
        var buf = Array(s.utf8CString)
        buf.withUnsafeMutableBufferPointer {
            GoviReplaceType(handle, r.0, Int32(r.1), r.2, Int32(r.3), $0.baseAddress)
        }
        clearSelection()
        step()
        return true
    }

    private func deleteSelection() {
        guard let r = bufferRangeForSelection() else { editBeep(); return }
        GoviDeleteRange(handle, r.0, Int32(r.1), r.2, Int32(r.3))
        clearSelection()
        step()
    }

    // MARK: - NSTextInputClient

    func insertText(_ string: Any, replacementRange: NSRange) {
        markedText = ""
        let s = Self.asString(string)
        if s.isEmpty { return }
        if replaceWithText(s) { return } // typed over a selection
        for scalar in s.unicodeScalars {
            GoviKeyRune(handle, Int32(scalar.value), 0)
        }
        step()
    }

    override func doCommand(by selector: Selector) {
        switch NSStringFromSelector(selector) {
        case "insertNewline:", "insertLineBreak:", "insertParagraphSeparator:":
            dispatchSpecialKey(SK.enter)
        case "insertTab:":
            dispatchTab()
        case "deleteBackward:":
            dispatchSpecialKey(SK.backspace)
        case "deleteForward:":
            dispatchSpecialKey(SK.delete)
        case "cancelOperation:":
            dispatchSpecialKey(SK.escape)
        case "moveUp:": dispatchSpecialKey(SK.up)
        case "moveDown:": dispatchSpecialKey(SK.down)
        case "moveLeft:": dispatchSpecialKey(SK.left)
        case "moveRight:": dispatchSpecialKey(SK.right)
        case "moveToBeginningOfLine:", "moveToBeginningOfParagraph:":
            dispatchSpecialKey(SK.home)
        case "moveToEndOfLine:", "moveToEndOfParagraph:":
            dispatchSpecialKey(SK.end)
        case "scrollPageUp:", "pageUp:", "pageUpAndModifySelection:":
            dispatchSpecialKey(SK.pageUp)
        case "scrollPageDown:", "pageDown:", "pageDownAndModifySelection:":
            dispatchSpecialKey(SK.pageDown)
        default:
            break // ignore Emacs-style bindings we don't want
        }
    }

    private func dispatchTab() {
        if selActive { clearSelection() }
        GoviKeyRune(handle, 9, 0)
        step()
    }

    private func sendSpecial(_ key: Int32) {
        if selActive { clearSelection() }
        GoviKeySpecial(handle, key, 0)
        step()
    }

    func setMarkedText(_ string: Any, selectedRange: NSRange, replacementRange: NSRange) {
        markedText = Self.asString(string)
        needsDisplay = true
    }

    func unmarkText() {
        if !markedText.isEmpty {
            markedText = ""
            needsDisplay = true
        }
    }

    func hasMarkedText() -> Bool { !markedText.isEmpty }

    func selectedRange() -> NSRange { NSRange(location: NSNotFound, length: 0) }

    func markedRange() -> NSRange {
        markedText.isEmpty
            ? NSRange(location: NSNotFound, length: 0)
            : NSRange(location: 0, length: markedText.utf16.count)
    }

    func attributedSubstring(forProposedRange range: NSRange, actualRange: NSRangePointer?) -> NSAttributedString? {
        nil
    }

    func validAttributesForMarkedText() -> [NSAttributedString.Key] { [] }

    func characterIndex(for point: NSPoint) -> Int { NSNotFound }

    // firstRect tells the input system where the cursor is (in screen
    // coordinates) so the dead-key/IME candidate window appears there.
    func firstRect(forCharacterRange range: NSRange, actualRange: NSRangePointer?) -> NSRect {
        let r = cellRect(Int(GoviCursorX(handle)), Int(GoviCursorY(handle)))
        let inWindow = convert(r, to: nil)
        return window?.convertToScreen(inWindow) ?? r
    }

    private static func asString(_ v: Any) -> String {
        if let s = v as? String { return s }
        if let a = v as? NSAttributedString { return a.string }
        if let n = v as? NSString { return n as String }
        return ""
    }

    // MARK: - Spell checking

    private func lineText(_ line: Int64) -> String {
        guard let c = GoviLineText(handle, line) else { return "" }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    // updateSpelling recomputes the misspelled words on the visible lines.
    private func updateSpelling() {
        misspellings.removeAll()
        guard spellEnabled else { return }
        // In ex (Q) mode the window shows a transcript, not the buffer, so
        // buffer-anchored squiggles would be misplaced.
        if GoviExActive(handle) != 0 { return }
        let checker = NSSpellChecker.shared
        let count = GoviLineCount(handle)
        var line = GoviTopLine(handle)
        while line <= count {
            var x: Int32 = 0, y: Int32 = 0, vis: Int32 = 0
            GoviPosToCell(handle, line, 0, &x, &y, &vis)
            if vis == 0 { break } // first off-screen line ends the visible range
            for r in misspelledRanges(lineText(line), checker) {
                misspellings.append(Misspelling(line: line, start: r.start, end: r.end))
            }
            line += 1
        }
    }

    private func misspelledRanges(_ text: String, _ checker: NSSpellChecker) -> [RuneRange] {
        if let cached = spellCache[text] { return cached }
        var result: [RuneRange] = []
        let len = (text as NSString).length
        var start = 0
        while start < len {
            let r = checker.checkSpelling(of: text, startingAt: start, language: nil,
                                          wrap: false, inSpellDocumentWithTag: spellTag, wordCount: nil)
            if r.location == NSNotFound || r.length == 0 { break }
            if let rr = runeRange(text, r) { result.append(rr) }
            start = r.location + r.length
        }
        spellCache[text] = result
        if spellCache.count > 4000 { spellCache.removeAll() }
        return result
    }

    // runeRange converts an NSRange (UTF-16) into rune (Unicode scalar) indices,
    // which is what the engine uses for columns.
    private func runeRange(_ s: String, _ ns: NSRange) -> RuneRange? {
        guard let range = Range(ns, in: s) else { return nil }
        let sc = s.unicodeScalars
        return (sc.distance(from: sc.startIndex, to: range.lowerBound),
                sc.distance(from: sc.startIndex, to: range.upperBound))
    }

    private func cellOf(_ line: Int64, _ col: Int) -> (x: Int, y: Int, vis: Bool) {
        var x: Int32 = 0, y: Int32 = 0, v: Int32 = 0
        GoviPosToCell(handle, line, Int32(col), &x, &y, &v)
        return (Int(x), Int(y), v != 0)
    }

    private func drawSpelling() {
        NSColor.systemRed.setStroke()
        for m in misspellings where m.end > m.start {
            let a = cellOf(m.line, m.start)
            let b = cellOf(m.line, m.end - 1)
            if a.vis && b.vis && a.y == b.y {
                squiggle(row: a.y, fromCol: a.x, toCol: b.x + 1)
            } else {
                // Wrapped or partially scrolled word: underline per row.
                var byRow: [Int: (Int, Int)] = [:]
                for r in m.start..<m.end {
                    let c = cellOf(m.line, r)
                    if !c.vis { continue }
                    if let e = byRow[c.y] {
                        byRow[c.y] = (min(e.0, c.x), max(e.1, c.x))
                    } else {
                        byRow[c.y] = (c.x, c.x)
                    }
                }
                for (row, span) in byRow {
                    squiggle(row: row, fromCol: span.0, toCol: span.1 + 1)
                }
            }
        }
    }

    private func squiggle(row: Int, fromCol: Int, toCol: Int) {
        let x0 = padding + CGFloat(fromCol) * cellW
        let x1 = padding + CGFloat(toCol) * cellW
        let yB = padding + CGFloat(row) * cellH + cellH - 1.5
        let path = NSBezierPath()
        let amp: CGFloat = 1.5
        let stepX: CGFloat = 2
        var x = x0
        var up = true
        path.move(to: NSPoint(x: x, y: yB))
        while x < x1 {
            x = min(x + stepX, x1)
            path.line(to: NSPoint(x: x, y: yB - (up ? amp : 0)))
            up.toggle()
        }
        path.lineWidth = 1
        path.stroke()
    }

    // MARK: - Context menu (right-click / control-click)

    private struct LookUpContext { let text: String; let x: Int32; let y: Int32 }

    // menu(for:) is called by AppKit for both right-click and control-click. It
    // builds a standard text context menu: spelling suggestions (when the word
    // is misspelled), a dictionary Look Up, and Cut/Copy/Paste. The word under
    // the click is selected so those commands act on it.
    override func menu(for event: NSEvent) -> NSMenu? {
        window?.makeFirstResponder(self)
        let cell = cellAt(event)
        var line: Int64 = 0, col: Int32 = 0
        GoviCellToPos(handle, cell.x, cell.y, &line, &col)

        let word = wordAt(cell: cell)
        if !word.text.isEmpty {
            if word.buffer {
                setBufferSelection(word.bufStart, word.bufEnd)
            } else {
                setScreenSelection(.linearScreen, word.screenStart, word.screenEnd)
            }
            step()
        }

        let menu = NSMenu()
        if let m = misspelling(at: line, col: Int(col)) {
            contextMisspelling = m
            addSuggestions(to: menu, for: m)
        }
        if !word.text.isEmpty {
            let look = menu.addItem(withTitle: "Look Up “\(word.text)”",
                                    action: #selector(lookUp(_:)), keyEquivalent: "")
            look.target = self
            look.representedObject = LookUpContext(text: word.text, x: cell.x, y: cell.y)
            menu.addItem(.separator())
        }
        for (title, sel) in [("Cut", #selector(cut(_:))), ("Copy", #selector(copy(_:))),
                             ("Paste", #selector(paste(_:)))] {
            let item = menu.addItem(withTitle: title, action: sel, keyEquivalent: "")
            item.target = self
        }
        return menu
    }

    // wordAt returns the word at a screen cell. Uses buffer word boundaries in the
    // editor; screen-row boundaries on overlay/ex or non-buffer rows.
    private func wordAt(cell c: (x: Int32, y: Int32))
        -> (buffer: Bool, bufStart: Caret, bufEnd: Caret,
            screenStart: ScreenCell, screenEnd: ScreenCell, text: String) {
        var line: Int64 = 0, col: Int32 = 0
        GoviCellToPos(handle, c.x, c.y, &line, &col)
        var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
        GoviWordRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
        let bufText = bridgeString(GoviRangeText(handle, l1, c1, l2, c2))
        if GoviOverlayActive(handle) == 0 && GoviExActive(handle) == 0
            && GoviScreenToBuffer(handle, c.x, c.y, &line, &col) != 0 && !bufText.isEmpty {
            return (true, (l1, Int(c1)), (l2, Int(c2)), (0, 0), (0, 0), bufText)
        }
        let (start, end) = screenWordRange(at: (c.x, c.y))
        guard let row = screenRowText(Int(c.y)) else {
            return (false, (1, 0), (1, 0), start, end, "")
        }
        let chars = Array(row)
        let s = Int(start.x), e = Int(end.x)
        guard s < e, e <= chars.count else {
            return (false, (1, 0), (1, 0), start, end, "")
        }
        return (false, (1, 0), (1, 0), start, end, String(chars[s..<e]))
    }

    private func misspelling(at line: Int64, col: Int) -> Misspelling? {
        misspellings.first { $0.line == line && col >= $0.start && col < $0.end }
    }

    private func word(_ m: Misspelling) -> String {
        let sc = Array(lineText(m.line).unicodeScalars)
        guard m.start >= 0, m.end <= sc.count, m.start < m.end else { return "" }
        var v = String.UnicodeScalarView()
        v.append(contentsOf: sc[m.start..<m.end])
        return String(v)
    }

    private func addSuggestions(to menu: NSMenu, for m: Misspelling) {
        let w = word(m)
        let guesses = NSSpellChecker.shared.guesses(
            forWordRange: NSRange(location: 0, length: (w as NSString).length),
            in: w, language: nil, inSpellDocumentWithTag: spellTag) ?? []
        if guesses.isEmpty {
            menu.addItem(withTitle: "No Guesses Found", action: nil, keyEquivalent: "").isEnabled = false
        } else {
            for g in guesses {
                let item = menu.addItem(withTitle: g, action: #selector(replaceSpelling(_:)), keyEquivalent: "")
                item.target = self
                item.representedObject = g
            }
        }
        menu.addItem(.separator())
        let ignore = menu.addItem(withTitle: "Ignore Spelling", action: #selector(ignoreSpelling(_:)), keyEquivalent: "")
        ignore.target = self
        let learn = menu.addItem(withTitle: "Learn Spelling", action: #selector(learnSpelling(_:)), keyEquivalent: "")
        learn.target = self
        menu.addItem(.separator())
    }

    // lookUp shows the system Dictionary popover for the word, anchored at the
    // clicked cell.
    @objc private func lookUp(_ sender: NSMenuItem) {
        guard let ctx = sender.representedObject as? LookUpContext else { return }
        let origin = cellPoint(Int(ctx.x), Int(ctx.y))
        let baseline = NSPoint(x: origin.x, y: origin.y + font.ascender)
        showDefinition(for: NSAttributedString(string: ctx.text), at: baseline)
    }

    @objc private func replaceSpelling(_ sender: NSMenuItem) {
        guard let m = contextMisspelling, let g = sender.representedObject as? String else { return }
        var buf = Array(g.utf8CString)
        buf.withUnsafeMutableBufferPointer {
            GoviReplaceText(handle, m.line, Int32(m.start), m.line, Int32(m.end), $0.baseAddress)
        }
        clearSelection()
        step()
    }

    @objc private func ignoreSpelling(_ sender: Any?) {
        guard let m = contextMisspelling else { return }
        NSSpellChecker.shared.ignoreWord(word(m), inSpellDocumentWithTag: spellTag)
        spellCache.removeAll()
        step()
    }

    @objc private func learnSpelling(_ sender: Any?) {
        guard let m = contextMisspelling else { return }
        NSSpellChecker.shared.learnWord(word(m))
        spellCache.removeAll()
        step()
    }

    // MARK: - Drawing

    override func draw(_ dirtyRect: NSRect) {
        bgColor.setFill()
        bounds.fill()

        let n = Int(GoviRows(handle))

        // Selection highlight: fill the background of reverse-video cells before
        // painting glyphs over them.
        if selActive {
            NSColor.selectedTextBackgroundColor.setFill()
            for y in 0..<n {
                guard let st = rowStyle(y) else { continue }
                for (x, flag) in st.enumerated() where flag == "1" {
                    cellRect(x, y).fill()
                }
            }
        }

        for y in 0..<n {
            drawRow(y)
        }

        if !misspellings.isEmpty {
            drawSpelling()
        }

        // Marked (uncommitted) text from a dead key/IME: draw it underlined at
        // the cursor and skip the block cursor while it is pending.
        if !markedText.isEmpty {
            let cx = Int(GoviCursorX(handle))
            let cy = Int(GoviCursorY(handle))
            let attrs: [NSAttributedString.Key: Any] = [
                .font: font, .foregroundColor: fgColor,
                .underlineStyle: NSUnderlineStyle.single.rawValue,
            ]
            for (i, ch) in markedText.enumerated() {
                let r = cellRect(cx + i, cy)
                bgColor.setFill()
                r.fill()
                (String(ch) as NSString).draw(at: r.origin, withAttributes: attrs)
            }
        } else if GoviCursorVisible(handle) != 0 {
            let cx = Int(GoviCursorX(handle))
            let cy = Int(GoviCursorY(handle))
            cursorColor.setFill()
            cellRect(cx, cy).fill()
            if let ch = charAt(cx, cy), ch != " " {
                drawChar(ch, col: cx, row: cy, color: bgColor)
            }
        }
    }

    private func drawRow(_ y: Int) {
        guard let s = screenRowText(y) else { return }
        for (i, ch) in s.enumerated() where ch != " " {
            drawChar(ch, col: i, row: y, color: fgColor)
        }
    }

    private func drawChar(_ ch: Character, col: Int, row: Int, color: NSColor) {
        let attrs: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: color]
        (String(ch) as NSString).draw(at: cellPoint(col, row), withAttributes: attrs)
    }

    private func screenRowText(_ y: Int) -> String? {
        guard let c = GoviRowText(handle, Int32(y)) else { return nil }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    private func rowStyle(_ y: Int) -> String? {
        guard let c = GoviRowStyle(handle, Int32(y)) else { return nil }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    private func charAt(_ col: Int, _ row: Int) -> Character? {
        guard let s = screenRowText(row) else { return nil }
        let arr = Array(s)
        return col < arr.count ? arr[col] : " "
    }
}
