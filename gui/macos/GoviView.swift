import Cocoa

// GoviView is the editor surface: a custom NSView that draws the engine's
// composed character grid and forwards key events into the embedded engine.
// The engine runs in-process (via libgovi); this view is the "terminal" it
// renders to.
final class GoviView: NSView, NSTextInputClient {
    // Engine-event modifier bits (must match engine.Mod).
    private static let modCtrl: Int32 = 1
    private static let modAlt: Int32 = 2

    // Pending marked (uncommitted) text from a dead key or IME, e.g. the "¨"
    // after Option-u, shown at the cursor until the next key composes it.
    private var markedText = ""

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

    // keyDown splits input two ways. Control-modified keys are vi commands and
    // are translated directly, so Cocoa's built-in Emacs-style key bindings
    // (which would turn ^F/^B/^A/... into cursor motions) never see them.
    // Everything else is handed to the text input system via interpretKeyEvents
    // so plain typing, Option-accents (Option-o -> o-slash), dead keys
    // (Option-u then u -> u-umlaut), and IMEs compose correctly; the composed
    // result arrives back through the NSTextInputClient methods below.
    override func keyDown(with event: NSEvent) {
        if event.modifierFlags.contains(.control) {
            handleControlKey(event)
            return
        }
        interpretKeyEvents([event])
    }

    private func handleControlKey(_ event: NSEvent) {
        if selActive { clearSelection() } // a command cancels the selection
        var mods: Int32 = GoviView.modCtrl
        if event.modifierFlags.contains(.option) { mods |= GoviView.modAlt }
        guard let chars = event.charactersIgnoringModifiers, !chars.isEmpty else { return }
        for scalar in chars.unicodeScalars {
            GoviKeyRune(Int32(scalar.value), mods)
        }
        step()
    }

    // replaceWithText replaces the active selection with s and returns true, or
    // returns false if there was no selection (GUI replace-on-type / paste).
    private func replaceWithText(_ s: String) -> Bool {
        guard selActive else { return false }
        var buf = Array(s.utf8CString)
        buf.withUnsafeMutableBufferPointer {
            GoviReplaceType(selStart.line, Int32(selStart.col),
                            selEnd.line, Int32(selEnd.col), $0.baseAddress)
        }
        clearSelection()
        step()
        return true
    }

    private func deleteSelection() {
        GoviDeleteRange(selStart.line, Int32(selStart.col), selEnd.line, Int32(selEnd.col))
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
            GoviKeyRune(Int32(scalar.value), 0)
        }
        step()
    }

    override func doCommand(by selector: Selector) {
        switch NSStringFromSelector(selector) {
        case "insertNewline:", "insertLineBreak:", "insertParagraphSeparator:":
            if replaceWithText("\n") { return }
            sendSpecial(SK.enter)
        case "insertTab:":
            if selActive { clearSelection() }
            GoviKeyRune(9, 0)
            step()
        case "deleteBackward:":
            if selActive { deleteSelection(); return }
            sendSpecial(SK.backspace)
        case "deleteForward:":
            if selActive { deleteSelection(); return }
            sendSpecial(SK.delete)
        case "cancelOperation:":
            if selActive { clearSelection() }
            sendSpecial(SK.escape)
        case "moveUp:": sendSpecial(SK.up)
        case "moveDown:": sendSpecial(SK.down)
        case "moveLeft:": sendSpecial(SK.left)
        case "moveRight:": sendSpecial(SK.right)
        case "moveToBeginningOfLine:", "moveToBeginningOfParagraph:": sendSpecial(SK.home)
        case "moveToEndOfLine:", "moveToEndOfParagraph:": sendSpecial(SK.end)
        case "scrollPageUp:", "pageUp:", "pageUpAndModifySelection:": sendSpecial(SK.pageUp)
        case "scrollPageDown:", "pageDown:", "pageDownAndModifySelection:": sendSpecial(SK.pageDown)
        default:
            break // ignore Emacs-style bindings we don't want
        }
    }

    private func sendSpecial(_ key: Int32) {
        if selActive { clearSelection() }
        GoviKeySpecial(key, 0)
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
        let r = NSRect(x: CGFloat(GoviCursorX()) * cellW, y: CGFloat(GoviCursorY()) * cellH,
                       width: cellW, height: cellH)
        let inWindow = convert(r, to: nil)
        return window?.convertToScreen(inWindow) ?? r
    }

    private static func asString(_ v: Any) -> String {
        if let s = v as? String { return s }
        if let a = v as? NSAttributedString { return a.string }
        if let n = v as? NSString { return n as String }
        return ""
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

        // Marked (uncommitted) text from a dead key/IME: draw it underlined at
        // the cursor and skip the block cursor while it is pending.
        if !markedText.isEmpty {
            let cx = Int(GoviCursorX())
            let cy = Int(GoviCursorY())
            let attrs: [NSAttributedString.Key: Any] = [
                .font: font, .foregroundColor: fgColor,
                .underlineStyle: NSUnderlineStyle.single.rawValue,
            ]
            for (i, ch) in markedText.enumerated() {
                let r = NSRect(x: CGFloat(cx + i) * cellW, y: CGFloat(cy) * cellH,
                               width: cellW, height: cellH)
                bgColor.setFill()
                r.fill()
                (String(ch) as NSString).draw(at: r.origin, withAttributes: attrs)
            }
        } else if GoviCursorVisible() != 0 {
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
