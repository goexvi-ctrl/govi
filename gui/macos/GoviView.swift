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

    // Pending marked (uncommitted) text from a dead key or IME, e.g. the "¨"
    // after Option-u, shown at the cursor until the next key composes it.
    private var markedText = ""

    private let font = NSFont.monospacedSystemFont(ofSize: 14, weight: .regular)
    private var cellW: CGFloat = 8
    private var cellH: CGFloat = 16

    private let bgColor = NSColor.textBackgroundColor
    private let fgColor = NSColor.textColor
    private let cursorColor = NSColor.systemBlue

    // Inset (pixels) between the window edge and the text grid, from Settings.
    private var padding: CGFloat = Settings.padding

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

    @objc private func settingsChanged() {
        padding = Settings.padding
        spellEnabled = Settings.spellChecking
        updateGeometry() // padding may change the cell rows/cols
        updateSpelling()
        needsDisplay = true
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
        let w = max(1, Int((bounds.width - 2 * padding) / cellW))
        let h = max(1, Int((bounds.height - 2 * padding) / cellH))
        if w == cols && h == rows { return }
        cols = w
        rows = h
        GoviResize(handle, Int32(rows), Int32(cols))
        recompose()
        needsDisplay = true
    }

    // MARK: - Engine step

    // step is run after every input: it handles quit, bell, title, recomposes
    // the grid, repaints, and arms any pending timer (map/showmatch/recovery).
    func step() {
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
        updateSpelling()
    }

    func updateTitle() {
        guard let c = GoviTitle(handle) else { return }
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

    private func caretAt(_ event: NSEvent) -> Caret {
        let c = cellAt(event)
        var line: Int64 = 0
        var col: Int32 = 0
        GoviCellToPos(handle, c.x, c.y, &line, &col)
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
        GoviSetSelection(handle, 1, a.line, Int32(a.col), b.line, Int32(b.col))
    }

    private func clearSelection() {
        if !selActive { return }
        selActive = false
        GoviSetSelection(handle, 0, 0, 0, 0, 0)
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
                GoviWordRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
            } else {
                GoviLineRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
            }
            dragging = false
            setSelection((l1, Int(c1)), (l2, Int(c2)))
            GoviMoveCursor(handle, l2, c2)
            step()
            return
        }

        let caret = caretAt(event)
        dragAnchor = caret
        dragging = true
        clearSelection()
        GoviMoveCursor(handle, caret.line, Int32(caret.col))
        step()
    }

    override func mouseDragged(with event: NSEvent) {
        guard dragging else { return }
        let caret = caretAt(event)
        setSelection(dragAnchor, caret)
        GoviMoveCursor(handle, caret.line, Int32(caret.col))
        step()
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
        let s = bridgeString(GoviRangeText(handle, selStart.line, Int32(selStart.col),
                                           selEnd.line, Int32(selEnd.col)))
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(s, forType: .string)
    }

    @objc func cut(_ sender: Any?) {
        guard selActive else { return }
        copy(sender)
        GoviDeleteRange(handle, selStart.line, Int32(selStart.col), selEnd.line, Int32(selEnd.col))
        clearSelection()
        step()
    }

    @objc func paste(_ sender: Any?) {
        guard let s = NSPasteboard.general.string(forType: .string) else { return }
        var buf = Array(s.utf8CString)
        if selActive {
            buf.withUnsafeMutableBufferPointer {
                GoviReplaceText(handle, selStart.line, Int32(selStart.col),
                                selEnd.line, Int32(selEnd.col), $0.baseAddress)
            }
            clearSelection()
        } else {
            buf.withUnsafeMutableBufferPointer { GoviInsertText(handle, $0.baseAddress) }
        }
        step()
    }

    @objc override func selectAll(_ sender: Any?) {
        var line: Int64 = 0
        var col: Int32 = 0
        GoviEndPos(handle, &line, &col)
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
            GoviKeyRune(handle, Int32(scalar.value), mods)
        }
        step()
    }

    // replaceWithText replaces the active selection with s and returns true, or
    // returns false if there was no selection (GUI replace-on-type / paste).
    private func replaceWithText(_ s: String) -> Bool {
        guard selActive else { return false }
        var buf = Array(s.utf8CString)
        buf.withUnsafeMutableBufferPointer {
            GoviReplaceType(handle, selStart.line, Int32(selStart.col),
                            selEnd.line, Int32(selEnd.col), $0.baseAddress)
        }
        clearSelection()
        step()
        return true
    }

    private func deleteSelection() {
        GoviDeleteRange(handle, selStart.line, Int32(selStart.col), selEnd.line, Int32(selEnd.col))
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
            if replaceWithText("\n") { return }
            sendSpecial(SK.enter)
        case "insertTab:":
            if selActive { clearSelection() }
            GoviKeyRune(handle, 9, 0)
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
            setSelection(word.start, word.end) // right-click selects the word
            needsDisplay = true
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

    // wordAt returns the word the engine's word boundary finds at a screen cell,
    // as caret endpoints and text. Empty text means the click was not on a word.
    private func wordAt(cell c: (x: Int32, y: Int32)) -> (start: Caret, end: Caret, text: String) {
        var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
        GoviWordRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
        let text = bridgeString(GoviRangeText(handle, l1, c1, l2, c2))
        return ((l1, Int(c1)), (l2, Int(c2)), text)
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
        guard let s = rowText(y) else { return }
        for (i, ch) in s.enumerated() where ch != " " {
            drawChar(ch, col: i, row: y, color: fgColor)
        }
    }

    private func drawChar(_ ch: Character, col: Int, row: Int, color: NSColor) {
        let attrs: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: color]
        (String(ch) as NSString).draw(at: cellPoint(col, row), withAttributes: attrs)
    }

    private func rowText(_ y: Int) -> String? {
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
        guard let s = rowText(row) else { return nil }
        let arr = Array(s)
        return col < arr.count ? arr[col] : " "
    }
}
