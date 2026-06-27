import AppKit

// FontPreview lays out ASCII 0x20…0x7e in a 16-column grid (6 rows; the last has 15).
enum FontPreview {
    static let columns = 16
    static let text = String((0x20...0x7e).compactMap { UnicodeScalar($0).map(Character.init) })

    static var rows: [String] {
        var lines: [String] = []
        var idx = text.startIndex
        while idx < text.endIndex {
            let end = text.index(idx, offsetBy: columns, limitedBy: text.endIndex) ?? text.endIndex
            lines.append(String(text[idx..<end]))
            idx = end
        }
        return lines
    }
}

// FontPreviewView draws the charset grid with the current font and spacing.
final class FontPreviewView: NSView {
    var font: NSFont = Settings.editorFont
    var characterSpacing: CGFloat = Settings.characterSpacing
    var lineSpacing: CGFloat = Settings.lineSpacing

    override var isFlipped: Bool { true }

    override func draw(_ dirtyRect: NSRect) {
        NSColor.textBackgroundColor.setFill()
        bounds.fill()

        let metrics = Settings.cellSize(
            font: font, characterSpacing: characterSpacing, lineSpacing: lineSpacing)
        let attrs: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: NSColor.textColor]
        for (row, line) in FontPreview.rows.enumerated() {
            for (col, ch) in line.enumerated() {
                let origin = NSPoint(
                    x: CGFloat(col) * metrics.cellW,
                    y: CGFloat(row) * metrics.cellH)
                (String(ch) as NSString).draw(at: origin, withAttributes: attrs)
            }
        }
    }

    func preferredSize() -> NSSize {
        let metrics = Settings.cellSize(
            font: font, characterSpacing: characterSpacing, lineSpacing: lineSpacing)
        return NSSize(
            width: ceil(CGFloat(FontPreview.columns) * metrics.cellW) + 16,
            height: ceil(CGFloat(FontPreview.rows.count) * metrics.cellH) + 16)
    }
}

// FontSettingsPanelController is the Change… sheet for font, size, and spacing.
final class FontSettingsPanelController: NSWindowController,
    NSTableViewDataSource, NSTableViewDelegate, NSTextFieldDelegate
{
    private let tableView = NSTableView()
    private let scrollView = NSScrollView()
    private let previewView = FontPreviewView()
    private let previewBox = NSBox()
    private let sizePopup = NSPopUpButton()
    private let characterSpacingSlider = NSSlider()
    private let lineSpacingSlider = NSSlider()
    private let characterSpacingValue = NSTextField(string: "1")
    private let lineSpacingValue = NSTextField(string: "1")

    private var draftPostScriptName = Settings.fontPostScriptName
    private var draftFontSize = Settings.fontSize
    private var draftCharacterSpacing = Settings.characterSpacing
    private var draftLineSpacing = Settings.lineSpacing

    // Retain the controller for the sheet lifetime; otherwise targets/delegates die
    // when show() returns and every control beeps or ignores input.
    private static var activeSheet: FontSettingsPanelController?

    private init() {
        let win = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 620, height: 420),
            styleMask: [.titled, .closable],
            backing: .buffered,
            defer: false)
        win.title = "Font"
        win.isReleasedWhenClosed = false
        super.init(window: win)
        buildUI()
        loadDraftFromSettings()
    }

    required init?(coder: NSCoder) { fatalError("not supported") }

    static func show(relativeTo parent: NSWindow, onClose: @escaping () -> Void) {
        let controller = FontSettingsPanelController()
        activeSheet = controller
        parent.beginSheet(controller.window!) { _ in
            activeSheet = nil
            onClose()
        }
    }

    private func buildUI() {
        guard let content = window?.contentView else { return }

        let sizeLabel = NSTextField(labelWithString: "Size:")
        sizeLabel.translatesAutoresizingMaskIntoConstraints = false
        sizePopup.translatesAutoresizingMaskIntoConstraints = false
        sizePopup.autoenablesItems = false
        sizePopup.target = self
        sizePopup.action = #selector(sizeChanged)
        for size in Int(Settings.minFontSize)...Int(Settings.maxFontSize) {
            sizePopup.addItem(withTitle: "\(size)")
        }

        let sizeRow = NSStackView(views: [sizeLabel, sizePopup])
        sizeRow.translatesAutoresizingMaskIntoConstraints = false
        sizeRow.alignment = .centerY
        sizeRow.spacing = 8

        let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("font"))
        column.title = ""
        column.width = 200
        tableView.addTableColumn(column)
        tableView.headerView = nil
        tableView.rowHeight = 20
        tableView.usesAlternatingRowBackgroundColors = true
        tableView.columnAutoresizingStyle = .firstColumnOnlyAutoresizingStyle
        tableView.dataSource = self
        tableView.delegate = self
        tableView.target = self
        tableView.doubleAction = #selector(confirm)

        scrollView.translatesAutoresizingMaskIntoConstraints = false
        scrollView.documentView = tableView
        scrollView.hasVerticalScroller = true
        scrollView.borderType = .bezelBorder

        previewBox.translatesAutoresizingMaskIntoConstraints = false
        previewBox.titlePosition = .noTitle
        previewBox.boxType = .custom
        previewBox.fillColor = .textBackgroundColor
        previewBox.wantsLayer = true
        previewBox.layer?.borderWidth = 1
        previewBox.layer?.borderColor = NSColor.separatorColor.cgColor

        previewView.translatesAutoresizingMaskIntoConstraints = false
        previewBox.addSubview(previewView)

        let characterLabel = NSTextField(labelWithString: "Character spacing:")
        characterLabel.translatesAutoresizingMaskIntoConstraints = false
        characterSpacingSlider.translatesAutoresizingMaskIntoConstraints = false
        characterSpacingSlider.minValue = Double(Settings.minSpacing)
        characterSpacingSlider.maxValue = Double(Settings.maxSpacing)
        characterSpacingSlider.target = self
        characterSpacingSlider.action = #selector(characterSpacingChanged)
        configureSpacingField(characterSpacingValue)

        let lineLabel = NSTextField(labelWithString: "Line spacing:")
        lineLabel.translatesAutoresizingMaskIntoConstraints = false
        lineSpacingSlider.translatesAutoresizingMaskIntoConstraints = false
        lineSpacingSlider.minValue = Double(Settings.minSpacing)
        lineSpacingSlider.maxValue = Double(Settings.maxSpacing)
        lineSpacingSlider.target = self
        lineSpacingSlider.action = #selector(lineSpacingChanged)
        configureSpacingField(lineSpacingValue)

        let spacingGrid = NSGridView(views: [
            [characterLabel, characterSpacingSlider, characterSpacingValue],
            [lineLabel, lineSpacingSlider, lineSpacingValue],
        ])
        spacingGrid.translatesAutoresizingMaskIntoConstraints = false
        spacingGrid.rowSpacing = 10
        spacingGrid.columnSpacing = 8
        spacingGrid.column(at: 0).xPlacement = .trailing
        spacingGrid.column(at: 1).xPlacement = .fill
        spacingGrid.column(at: 2).xPlacement = .leading
        spacingGrid.column(at: 2).width = 56

        let rightStack = NSStackView(views: [previewBox, spacingGrid])
        rightStack.translatesAutoresizingMaskIntoConstraints = false
        rightStack.orientation = .vertical
        rightStack.alignment = .leading
        rightStack.spacing = 12

        let split = NSStackView(views: [scrollView, rightStack])
        split.translatesAutoresizingMaskIntoConstraints = false
        split.orientation = .horizontal
        split.alignment = .top
        split.spacing = 16
        split.distribution = .fill

        let cancel = NSButton(title: "Cancel", target: self, action: #selector(cancel))
        let ok = NSButton(title: "OK", target: self, action: #selector(confirm))
        ok.keyEquivalent = "\r"
        cancel.translatesAutoresizingMaskIntoConstraints = false
        ok.translatesAutoresizingMaskIntoConstraints = false
        let buttons = NSStackView(views: [cancel, ok])
        buttons.translatesAutoresizingMaskIntoConstraints = false
        buttons.orientation = .horizontal
        buttons.spacing = 8

        let root = NSStackView(views: [sizeRow, split, buttons])
        root.translatesAutoresizingMaskIntoConstraints = false
        root.orientation = .vertical
        root.alignment = .leading
        root.spacing = 16
        content.addSubview(root)

        NSLayoutConstraint.activate([
            root.leadingAnchor.constraint(equalTo: content.leadingAnchor, constant: 20),
            root.trailingAnchor.constraint(equalTo: content.trailingAnchor, constant: -20),
            root.topAnchor.constraint(equalTo: content.topAnchor, constant: 20),
            root.bottomAnchor.constraint(equalTo: content.bottomAnchor, constant: -20),

            scrollView.widthAnchor.constraint(equalToConstant: 220),
            scrollView.heightAnchor.constraint(equalToConstant: 280),

            spacingGrid.widthAnchor.constraint(equalTo: previewBox.widthAnchor),

            previewView.leadingAnchor.constraint(equalTo: previewBox.leadingAnchor, constant: 8),
            previewView.topAnchor.constraint(equalTo: previewBox.topAnchor, constant: 8),
            previewView.trailingAnchor.constraint(lessThanOrEqualTo: previewBox.trailingAnchor, constant: -8),
            previewView.bottomAnchor.constraint(lessThanOrEqualTo: previewBox.bottomAnchor, constant: -8),
            previewBox.widthAnchor.constraint(greaterThanOrEqualToConstant: 280),
            previewBox.heightAnchor.constraint(equalToConstant: 200),
        ])
    }

    private func configureSpacingField(_ field: NSTextField) {
        field.translatesAutoresizingMaskIntoConstraints = false
        field.alignment = .right
        field.controlSize = .small
        field.delegate = self
    }

    private func loadDraftFromSettings() {
        draftPostScriptName = Settings.fontPostScriptName
        draftFontSize = Settings.fontSize
        draftCharacterSpacing = Settings.characterSpacing
        draftLineSpacing = Settings.lineSpacing
        syncControls()
    }

    private func syncControls() {
        tableView.reloadData()
        if let idx = EditorFonts.choices.firstIndex(where: { $0.postScriptName == draftPostScriptName }) {
            tableView.selectRowIndexes(IndexSet(integer: idx), byExtendingSelection: false)
            tableView.scrollRowToVisible(idx)
        }
        let sizeIdx = Int(draftFontSize) - Int(Settings.minFontSize)
        if sizeIdx >= 0, sizeIdx < sizePopup.numberOfItems {
            sizePopup.selectItem(at: sizeIdx)
        }
        characterSpacingSlider.doubleValue = Double(draftCharacterSpacing)
        lineSpacingSlider.doubleValue = Double(draftLineSpacing)
        updateSpacingFields()
        updatePreview()
    }

    private func updateSpacingFields() {
        characterSpacingValue.stringValue = formatSpacing(draftCharacterSpacing)
        lineSpacingValue.stringValue = formatSpacing(draftLineSpacing)
    }

    private func formatSpacing(_ value: CGFloat) -> String {
        var s = String(format: "%.3f", value)
        while s.contains(".") && (s.hasSuffix("0") || s.hasSuffix(".")) {
            s.removeLast()
        }
        return s
    }

    private func updatePreview() {
        previewView.font = EditorFonts.font(postScriptName: draftPostScriptName, size: draftFontSize)
        previewView.characterSpacing = draftCharacterSpacing
        previewView.lineSpacing = draftLineSpacing
        previewView.needsDisplay = true
    }

    private func clampSpacing(_ value: CGFloat) -> CGFloat {
        min(max(value, Settings.minSpacing), Settings.maxSpacing)
    }

    @objc private func sizeChanged() {
        draftFontSize = CGFloat(Int(sizePopup.titleOfSelectedItem ?? "") ?? Int(Settings.defaultFontSize))
        updatePreview()
    }

    @objc private func characterSpacingChanged() {
        draftCharacterSpacing = clampSpacing(CGFloat(characterSpacingSlider.doubleValue))
        updateSpacingFields()
        updatePreview()
    }

    @objc private func lineSpacingChanged() {
        draftLineSpacing = clampSpacing(CGFloat(lineSpacingSlider.doubleValue))
        updateSpacingFields()
        updatePreview()
    }

    func controlTextDidEndEditing(_ obj: Notification) {
        guard let field = obj.object as? NSTextField else { return }
        let text = field.stringValue.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let raw = Double(text).map({ CGFloat($0) }), raw > 0 else {
            NSSound.beep()
            updateSpacingFields()
            return
        }
        let clamped = clampSpacing(raw)
        switch field {
        case characterSpacingValue:
            draftCharacterSpacing = clamped
            characterSpacingSlider.doubleValue = Double(clamped)
        case lineSpacingValue:
            draftLineSpacing = clamped
            lineSpacingSlider.doubleValue = Double(clamped)
        default:
            return
        }
        updateSpacingFields()
        updatePreview()
    }

    @objc private func cancel() {
        window?.sheetParent?.endSheet(window!, returnCode: .cancel)
    }

    @objc private func confirm() {
        Settings.fontPostScriptName = draftPostScriptName
        Settings.fontSize = draftFontSize
        Settings.characterSpacing = draftCharacterSpacing
        Settings.lineSpacing = draftLineSpacing
        window?.sheetParent?.endSheet(window!, returnCode: .OK)
    }

    func numberOfRows(in tableView: NSTableView) -> Int {
        EditorFonts.choices.count
    }

    func tableView(_ tableView: NSTableView, viewFor tableColumn: NSTableColumn?, row: Int) -> NSView? {
        let choice = EditorFonts.choices[row]
        let cell = NSTextField(labelWithString: choice.displayName)
        cell.lineBreakMode = .byTruncatingTail
        return cell
    }

    func tableViewSelectionDidChange(_ notification: Notification) {
        let row = tableView.selectedRow
        guard row >= 0, row < EditorFonts.choices.count else { return }
        draftPostScriptName = EditorFonts.choices[row].postScriptName
        updatePreview()
    }
}