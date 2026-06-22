import Cocoa

// GoviView is the editor surface: a custom NSView that draws the engine's
// composed character grid and forwards key events into the embedded engine.
// The engine runs in-process (via libgovi); this view is the "terminal" it
// renders to.
final class GoviView: NSView {
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

    private let font = NSFont.monospacedSystemFont(ofSize: 14, weight: .regular)
    private var cellW: CGFloat = 8
    private var cellH: CGFloat = 16

    private let bgColor = NSColor.textBackgroundColor
    private let fgColor = NSColor.textColor
    private let cursorColor = NSColor.systemBlue

    private var rows = 1
    private var cols = 1
    private var timer: Timer?

    // Mouse selection state, in buffer caret coordinates (1-based line, rune
    // index). selActive mirrors the bridge's highlighted range.
    private typealias Caret = (line: Int64, col: Int)
    private var selActive = false
    private var selStart: Caret = (1, 0)
    private var selEnd: Caret = (1, 0)
    private var dragAnchor: Caret = (1, 0)
    private var dragging = false

    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        measureFont()
    }
    required init?(coder: NSCoder) {
        super.init(coder: coder)
        measureFont()
    }

    private func measureFont() {
        let attrs: [NSAttributedString.Key: Any] = [.font: font]
        cellW = ("0" as NSString).size(withAttributes: attrs).width
        let lm = NSLayoutManager()
        cellH = lm.defaultLineHeight(for: font)
    }

    // The grid's first row is at the top, so use a flipped coordinate system.
    override var isFlipped: Bool { true }
    override var acceptsFirstResponder: Bool { true }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        window?.makeFirstResponder(self)
        updateGeometry()
    }

    override func setFrameSize(_ newSize: NSSize) {
        super.setFrameSize(newSize)
        updateGeometry()
    }

    // updateGeometry recomputes the cell rows/cols for the current bounds and
    // tells the engine, then recomposes and repaints.
    func updateGeometry() {
        let w = max(1, Int(bounds.width / cellW))
        let h = max(1, Int(bounds.height / cellH))
        if w == cols && h == rows { return }
        cols = w
        rows = h
        GoviResize(Int32(rows), Int32(cols))
        recompose()
        needsDisplay = true
    }

    // MARK: - Engine step

    // step is run after every input: it handles quit, bell, title, recomposes
    // the grid, repaints, and arms any pending timer (map/showmatch/recovery).
    func step() {
        if GoviShouldQuit() != 0 {
            NSApp.terminate(nil)
            return
        }
        if GoviTakeBell() != 0 {
            NSSound.beep()
        }
        updateTitle()
        recompose()
        needsDisplay = true
        armTimer()
    }

    private func recompose() {
        GoviCompose(Int32(rows), Int32(cols))
    }

    private func updateTitle() {
        guard let c = GoviTitle() else { return }
        defer { GoviFree(c) }
        let t = String(cString: c)
        if !t.isEmpty {
            window?.title = t
        }
    }

    private func armTimer() {
        timer?.invalidate()
        timer = nil
        var interval: TimeInterval = 0
        var action: (() -> Void)?
        if GoviMatchPending() != 0 {
            interval = Double(GoviMatchTimeMS()) / 1000.0
            action = { GoviFireTimeout() }
        } else if GoviMapPending() != 0 {
            interval = 0.5
            action = { GoviFireTimeout() }
        } else if GoviNeedsRecoverySync() != 0 {
            interval = 2.0
            action = { GoviSyncRecovery() }
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
        return (Int32(p.x / cellW), Int32(p.y / cellH))
    }

    private func caretAt(_ event: NSEvent) -> Caret {
        let c = cellAt(event)
        var line: Int64 = 0
        var col: Int32 = 0
        GoviCellToPos(c.x, c.y, &line, &col)
        return (line, Int(col))
    }

    private func setSelection(_ a: Caret, _ b: Caret) {
        if a == b {
            clearSelection()
            return
        }
        selActive = true
        selStart = a
        selEnd = b
        GoviSetSelection(1, a.line, Int32(a.col), b.line, Int32(b.col))
    }

    private func clearSelection() {
        if !selActive { return }
        selActive = false
        GoviSetSelection(0, 0, 0, 0, 0)
    }

    override func mouseDown(with event: NSEvent) {
        window?.makeFirstResponder(self)

        // Double-click selects the word under the cursor; triple-click selects
        // the whole line. Both are computed by the engine (the word boundary is
        // pluggable there).
        if event.clickCount == 2 || event.clickCount == 3 {
            let c = cellAt(event)
            var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
            if event.clickCount == 2 {
                GoviWordRange(c.x, c.y, &l1, &c1, &l2, &c2)
            } else {
                GoviLineRange(c.x, c.y, &l1, &c1, &l2, &c2)
            }
            dragging = false
            setSelection((l1, Int(c1)), (l2, Int(c2)))
            GoviMoveCursor(l2, c2)
            step()
            return
        }

        let caret = caretAt(event)
        dragAnchor = caret
        dragging = true
        clearSelection()
        GoviMoveCursor(caret.line, Int32(caret.col))
        step()
    }

    override func mouseDragged(with event: NSEvent) {
        guard dragging else { return }
        let caret = caretAt(event)
        setSelection(dragAnchor, caret)
        GoviMoveCursor(caret.line, Int32(caret.col))
        step()
    }

    override func mouseUp(with event: NSEvent) {
        dragging = false
    }

    // MARK: - Clipboard (Edit menu / standard shortcuts)

    @objc func copy(_ sender: Any?) {
        guard selActive else { return }
        let s = bridgeString(GoviRangeText(selStart.line, Int32(selStart.col),
                                           selEnd.line, Int32(selEnd.col)))
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(s, forType: .string)
    }

    @objc func cut(_ sender: Any?) {
        guard selActive else { return }
        copy(sender)
        GoviDeleteRange(selStart.line, Int32(selStart.col), selEnd.line, Int32(selEnd.col))
        clearSelection()
        step()
    }

    @objc func paste(_ sender: Any?) {
        guard let s = NSPasteboard.general.string(forType: .string) else { return }
        var buf = Array(s.utf8CString)
        if selActive {
            buf.withUnsafeMutableBufferPointer {
                GoviReplaceText(selStart.line, Int32(selStart.col),
                                selEnd.line, Int32(selEnd.col), $0.baseAddress)
            }
            clearSelection()
        } else {
            buf.withUnsafeMutableBufferPointer { GoviInsertText($0.baseAddress) }
        }
        step()
    }

    @objc override func selectAll(_ sender: Any?) {
        var line: Int64 = 0
        var col: Int32 = 0
        GoviEndPos(&line, &col)
        setSelection((1, 0), (line, Int(col)))
        needsDisplay = true
    }

    private func bridgeString(_ c: UnsafeMutablePointer<CChar>?) -> String {
        guard let c = c else { return "" }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    // MARK: - Input

    private func isPrintable(_ e: NSEvent) -> Bool {
        let f = e.modifierFlags
        if f.contains(.command) || f.contains(.control) { return false }
        if e.specialKey != nil { return false }
        if e.keyCode == 53 { return false } // escape
        guard let c = e.charactersIgnoringModifiers, !c.isEmpty else { return false }
        for s in c.unicodeScalars where s.value < 0x20 { return false }
        return true
    }

    override func keyDown(with event: NSEvent) {
        // GUI-standard: with an active selection, typing replaces it (and you
        // keep typing in insert mode); Backspace/Delete removes it; any other
        // key cancels the selection and is then handled normally.
        if selActive {
            if isPrintable(event) {
                let s = event.characters ?? ""
                var buf = Array(s.utf8CString)
                buf.withUnsafeMutableBufferPointer {
                    GoviReplaceType(selStart.line, Int32(selStart.col),
                                    selEnd.line, Int32(selEnd.col), $0.baseAddress)
                }
                clearSelection()
                return step()
            }
            if let sk = event.specialKey, sk == .delete || sk == .deleteForward {
                GoviDeleteRange(selStart.line, Int32(selStart.col),
                                selEnd.line, Int32(selEnd.col))
                clearSelection()
                return step()
            }
            clearSelection()
        }

        let flags = event.modifierFlags
        var mods: Int32 = 0
        if flags.contains(.control) { mods |= GoviView.modCtrl }
        if flags.contains(.option) { mods |= GoviView.modAlt }

        if let sk = event.specialKey {
            switch sk {
            case .upArrow: GoviKeySpecial(SK.up, mods); return step()
            case .downArrow: GoviKeySpecial(SK.down, mods); return step()
            case .leftArrow: GoviKeySpecial(SK.left, mods); return step()
            case .rightArrow: GoviKeySpecial(SK.right, mods); return step()
            case .pageUp: GoviKeySpecial(SK.pageUp, mods); return step()
            case .pageDown: GoviKeySpecial(SK.pageDown, mods); return step()
            case .home: GoviKeySpecial(SK.home, mods); return step()
            case .end: GoviKeySpecial(SK.end, mods); return step()
            case .delete: GoviKeySpecial(SK.backspace, mods); return step()
            case .deleteForward: GoviKeySpecial(SK.delete, mods); return step()
            case .carriageReturn, .enter, .newline:
                GoviKeySpecial(SK.enter, mods); return step()
            case .tab:
                GoviKeyRune(9, mods); return step()
            default: break
            }
        }

        // Escape has no specialKey case; identify it by key code.
        if event.keyCode == 53 {
            GoviKeySpecial(SK.escape, mods)
            return step()
        }

        guard let chars = event.charactersIgnoringModifiers, !chars.isEmpty else { return }

        if flags.contains(.control) {
            // Ctrl-letter: send the base character carrying the Ctrl modifier so
            // the engine's key tables match (e.g. Ctrl-F -> 'f' + ModCtrl).
            for scalar in chars.unicodeScalars {
                GoviKeyRune(Int32(scalar.value), mods)
            }
            return step()
        }

        if chars.count == 1, let scalar = chars.unicodeScalars.first {
            GoviKeyRune(Int32(scalar.value), mods)
        } else {
            var buf = Array(chars.utf8CString)
            buf.withUnsafeMutableBufferPointer { GoviText($0.baseAddress) }
        }
        step()
    }

    // MARK: - Drawing

    override func draw(_ dirtyRect: NSRect) {
        bgColor.setFill()
        bounds.fill()

        let n = Int(GoviRows())

        // Selection highlight: fill the background of reverse-video cells before
        // painting glyphs over them.
        if selActive {
            NSColor.selectedTextBackgroundColor.setFill()
            for y in 0..<n {
                guard let st = rowStyle(y) else { continue }
                for (x, flag) in st.enumerated() where flag == "1" {
                    NSRect(x: CGFloat(x) * cellW, y: CGFloat(y) * cellH,
                           width: cellW, height: cellH).fill()
                }
            }
        }

        for y in 0..<n {
            drawRow(y)
        }

        if GoviCursorVisible() != 0 {
            let cx = Int(GoviCursorX())
            let cy = Int(GoviCursorY())
            let rect = NSRect(x: CGFloat(cx) * cellW, y: CGFloat(cy) * cellH,
                              width: cellW, height: cellH)
            cursorColor.setFill()
            rect.fill()
            if let ch = charAt(cx, cy), ch != " " {
                drawChar(ch, col: cx, row: cy, color: bgColor)
            }
        }
    }

    private func drawRow(_ y: Int) {
        guard let s = rowText(y) else { return }
        for (i, ch) in s.enumerated() where ch != " " {
            drawChar(ch, col: i, row: y, color: fgColor)
        }
    }

    private func drawChar(_ ch: Character, col: Int, row: Int, color: NSColor) {
        let attrs: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: color]
        let p = NSPoint(x: CGFloat(col) * cellW, y: CGFloat(row) * cellH)
        (String(ch) as NSString).draw(at: p, withAttributes: attrs)
    }

    private func rowText(_ y: Int) -> String? {
        guard let c = GoviRowText(Int32(y)) else { return nil }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    private func rowStyle(_ y: Int) -> String? {
        guard let c = GoviRowStyle(Int32(y)) else { return nil }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    private func charAt(_ col: Int, _ row: Int) -> Character? {
        guard let s = rowText(row) else { return nil }
        let arr = Array(s)
        return col < arr.count ? arr[col] : " "
    }
}
