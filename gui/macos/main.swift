import Cocoa

// Govi: a native macOS application with the govi (Go nvi) editor engine embedded
// in-process. The engine is linked in as a C archive (libgovi); this app drives
// it and renders its screen in a custom NSView. nvi is *embedded*, not exec'd.
//
// The app is multi-window: each EditorWindow owns its own engine instance
// (a libgovi handle). Settings.openFilesIn controls whether opened files start
// in a new window or as a tab in the frontmost window.

// EditorWindow owns one window, its GoviView, and the embedded engine behind it.
// Instances keep themselves alive in a static set until their window closes.
final class EditorWindow: NSObject, NSWindowDelegate {
    private static var windows: Set<EditorWindow> = []
    private static var cascadePoint = NSPoint.zero

    let window: NSWindow
    let view: GoviView
    var path: String // the file this window is editing ("" = untitled)

    // anyOpen reports whether any editor window exists.
    static var anyOpen: Bool { !windows.isEmpty }

    // make creates an editor for path (empty path = an empty buffer) without
    // presenting it. Returns nil if the file could not be opened.
    private static func make(path: String) -> EditorWindow? {
        let fg = Settings.defaultForegroundColorSpec
        let bg = Settings.defaultBackgroundColorSpec
        let handle = path.withCString { pathPtr in
            fg.withCString { fgPtr in
                bg.withCString { bgPtr in
                    GoviStart(UnsafeMutablePointer(mutating: pathPtr),
                              UnsafeMutablePointer(mutating: fgPtr),
                              UnsafeMutablePointer(mutating: bgPtr))
                }
            }
        }
        guard handle != 0 else {
            let alert = NSAlert()
            alert.messageText = "Could not open “\(path)”."
            alert.runModal()
            return nil
        }
        let w = EditorWindow(handle: handle, path: path)
        windows.insert(w)
        return w
    }

    // open presents path in a new standalone window.
    @discardableResult
    static func open(path: String) -> EditorWindow? {
        guard let w = make(path: path) else { return nil }
        w.showStandalone()
        return w
    }

    // existing returns the window already editing path, if any.
    private static func existing(path p: String) -> EditorWindow? {
        p.isEmpty ? nil : windows.first(where: { $0.path == p })
    }

    // openPaths opens one or more files according to Settings.openFilesIn. In tab
    // mode, files join the frontmost window's tab group (or start a new window
    // when none exists). In new-window mode, each file gets its own window.
    // Already-open files are focused in place rather than duplicated.
    static func openPaths(_ paths: [String]) {
        let normalized = paths.map { ($0 as NSString).standardizingPath }
        guard !normalized.isEmpty else { return }

        switch Settings.openFilesIn {
        case .newWindow:
            for p in normalized {
                if let w = existing(path: p) {
                    w.window.makeKeyAndOrderFront(nil)
                } else {
                    _ = open(path: p)
                }
            }
        case .tab:
            openAsTabs(normalized)
        }
    }

    // openAsTabs opens paths in the frontmost window's tab group. The first file
    // starts a new window when no anchor exists yet.
    private static func openAsTabs(_ paths: [String]) {
        var anchor = NSApp.keyWindow ?? windows.first?.window
        for p in paths {
            if let w = existing(path: p) {
                w.window.makeKeyAndOrderFront(nil)
                anchor = w.window
                continue
            }
            if let a = anchor {
                openTab(in: a, path: p)
            } else if let w = open(path: p) {
                anchor = w.window
            }
        }
    }

    // openTab presents path as a new tab in keyWindow's tab group, or as a
    // standalone window if there is no key window to tab into.
    static func openTab(in keyWindow: NSWindow?, path: String) {
        guard let w = make(path: path) else { return }
        if let key = keyWindow, key !== w.window {
            key.addTabbedWindow(w.window, ordered: .above)
            w.window.makeKeyAndOrderFront(nil)
            w.window.makeFirstResponder(w.view)
            w.view.updateGeometry()
            w.view.updateTitle()
        } else {
            w.showStandalone()
        }
    }

    private init(handle: Int64, path: String) {
        self.path = (path as NSString).standardizingPath
        let size = GoviView.contentSize(
            textRows: Settings.defaultTextRows, cols: Settings.defaultColumns)
        let frame = NSRect(x: 0, y: 0, width: size.width, height: size.height)
        window = NSWindow(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)
        view = GoviView(frame: frame, handle: handle)
        super.init()
        view.documentTitle = path.isEmpty ? "Untitled" : (path as NSString).lastPathComponent
        window.contentView = view
        window.delegate = self
        window.isReleasedWhenClosed = false
        // Native macOS tabbing: a shared identifier lets these windows be tabbed
        // together and dragged between windows; .automatic respects the user's
        // "prefer tabs" setting for Cmd-N while still allowing explicit tabs.
        window.tabbingIdentifier = "govi"
        window.tabbingMode = .automatic
    }

    private func showStandalone() {
        EditorWindow.cascadePoint = window.cascadeTopLeft(from: EditorWindow.cascadePoint)
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(view)
        view.updateGeometry()
        view.updateTitle()
    }

    func windowDidBecomeKey(_ notification: Notification) {
        view.syncWorkingDirectory()
        view.updateTitle()
    }

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        confirmClose()
    }

    func windowWillClose(_ notification: Notification) {
        GoviClose(view.handle)
        EditorWindow.windows.remove(self)
    }

    // confirmClose returns true when the window/tab may close. When
    // Settings.warnOnUnsavedClose is set and the buffer is modified, the user
    // is prompted to save, discard, or cancel.
    private func confirmClose() -> Bool {
        if !Settings.warnOnUnsavedClose || GoviModified(view.handle) == 0 {
            return true
        }

        let displayName = path.isEmpty ? "Untitled" : (path as NSString).lastPathComponent
        let alert = NSAlert()
        alert.messageText = "Do you want to save the changes made to “\(displayName)”?"
        alert.informativeText = "Your changes will be lost if you don't save them."
        alert.alertStyle = .warning
        alert.addButton(withTitle: "Save")
        alert.addButton(withTitle: "Don't Save")
        alert.addButton(withTitle: "Cancel")

        switch alert.runModal() {
        case .alertFirstButtonReturn:
            return saveForClose()
        case .alertSecondButtonReturn:
            return true
        default:
            GoviClearQuit(view.handle)
            return false
        }
    }

    private func saveForClose() -> Bool {
        var target = path
        if target.isEmpty {
            let panel = NSSavePanel()
            panel.canCreateDirectories = true
            panel.nameFieldStringValue = "Untitled"
            guard panel.runModal() == .OK, let url = panel.url else {
                GoviClearQuit(view.handle)
                return false
            }
            target = url.path
        }
        let saved = target.withCString { ptr in
            GoviSave(view.handle, UnsafeMutablePointer(mutating: ptr))
        } == 0
        if !saved {
            let alert = NSAlert()
            alert.messageText = "The document could not be saved."
            alert.informativeText = "“\(target)” could not be written."
            alert.alertStyle = .warning
            alert.runModal()
            GoviClearQuit(view.handle)
            return false
        }
        path = (target as NSString).standardizingPath
        view.documentTitle = (path as NSString).lastPathComponent
        view.updateTitle()
        return true
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        // Files passed by direct exec (Govi.app/Contents/MacOS/Govi file ...).
        // Files passed via `open`/Finder arrive through application(_:open:).
        let cwd = FileManager.default.currentDirectoryPath
        let paths = CommandLine.arguments.dropFirst().map { arg -> String in
            (arg as NSString).isAbsolutePath ? arg : "\(cwd)/\(arg)"
        }
        EditorWindow.openPaths(Array(paths))
        // If nothing was opened (no args and no open event), show an empty
        // window. Deferred so any launch-time open event is handled first.
        DispatchQueue.main.async {
            if !EditorWindow.anyOpen {
                EditorWindow.open(path: "")
            }
        }
        NSApp.activate(ignoringOtherApps: true)
    }

    // application(_:open:) is the open-documents Apple Event. macOS routes
    // `open -a Govi.app file ...` (and Finder double-clicks / drags) here --
    // delivering to the *running* instance when one exists, which is what makes
    // the command-line `govi` tool reuse a running app.
    func application(_ application: NSApplication, open urls: [URL]) {
        EditorWindow.openPaths(urls.filter { $0.isFileURL }.map { $0.path })
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    // File > New: an empty window.
    @objc func newWindow(_ sender: Any?) {
        EditorWindow.open(path: "")
    }

    // File > New Tab: an empty tab in the current window's tab group.
    @objc func newTab(_ sender: Any?) {
        EditorWindow.openTab(in: NSApp.keyWindow, path: "")
    }

    // The "+" button in the tab bar routes here through the responder chain.
    @objc func newWindowForTab(_ sender: Any?) {
        EditorWindow.openTab(in: NSApp.keyWindow, path: "")
    }

    // Settings… (Cmd-,)
    @objc func showSettings(_ sender: Any?) {
        SettingsWindowController.shared.show()
    }

    // Edit > Spelling > Check Spelling While Typing.
    @objc func toggleSpelling(_ sender: Any?) {
        Settings.spellChecking.toggle()
    }

    func validateMenuItem(_ item: NSMenuItem) -> Bool {
        if item.action == #selector(toggleSpelling(_:)) {
            item.state = Settings.spellChecking ? .on : .off
        }
        return true
    }

    // File > Open…: choose one or more files (placement follows Settings).
    @objc func openWindow(_ sender: Any?) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = true
        panel.begin { response in
            guard response == .OK else { return }
            EditorWindow.openPaths(panel.urls.map { $0.path })
        }
    }
}

// Build the main menu. The app and Edit items use standard responder-chain
// selectors; File > New/Open target the app delegate.
func makeMenu(target: AppDelegate) -> NSMenu {
    let mainMenu = NSMenu()
    let name = ProcessInfo.processInfo.processName

    // Application menu.
    let appItem = NSMenuItem()
    mainMenu.addItem(appItem)
    let appMenu = NSMenu()
    appItem.submenu = appMenu
    appMenu.addItem(withTitle: "About \(name)", action: nil, keyEquivalent: "")
    appMenu.addItem(NSMenuItem.separator())
    let settingsItem = appMenu.addItem(withTitle: "Settings…",
                                       action: #selector(AppDelegate.showSettings(_:)), keyEquivalent: ",")
    settingsItem.target = target
    appMenu.addItem(NSMenuItem.separator())
    appMenu.addItem(withTitle: "Quit \(name)",
                    action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")

    // File menu.
    let fileItem = NSMenuItem()
    mainMenu.addItem(fileItem)
    let fileMenu = NSMenu(title: "File")
    fileItem.submenu = fileMenu
    let newItem = fileMenu.addItem(withTitle: "New", action: #selector(AppDelegate.newWindow(_:)), keyEquivalent: "n")
    newItem.target = target
    let tabItem = fileMenu.addItem(withTitle: "New Tab", action: #selector(AppDelegate.newTab(_:)), keyEquivalent: "t")
    tabItem.target = target
    let openItem = fileMenu.addItem(withTitle: "Open…", action: #selector(AppDelegate.openWindow(_:)), keyEquivalent: "o")
    openItem.target = target
    fileMenu.addItem(NSMenuItem.separator())
    fileMenu.addItem(withTitle: "Close Window", action: #selector(NSWindow.performClose(_:)), keyEquivalent: "w")

    // Edit menu: Cut/Copy/Paste/Select All route through the responder chain to
    // GoviView's overrides.
    let editItem = NSMenuItem()
    mainMenu.addItem(editItem)
    let editMenu = NSMenu(title: "Edit")
    editItem.submenu = editMenu
    editMenu.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
    editMenu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
    editMenu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
    editMenu.addItem(NSMenuItem.separator())
    editMenu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
    editMenu.addItem(NSMenuItem.separator())
    let spellingItem = NSMenuItem(title: "Spelling", action: nil, keyEquivalent: "")
    let spellingMenu = NSMenu(title: "Spelling")
    spellingItem.submenu = spellingMenu
    let checkItem = spellingMenu.addItem(withTitle: "Check Spelling While Typing",
                                         action: #selector(AppDelegate.toggleSpelling(_:)), keyEquivalent: "")
    checkItem.target = target
    editMenu.addItem(spellingItem)

    // Window menu. AppKit fills it with the window list and, because the windows
    // use native tabbing, the tab commands (Show Tab Bar, Show All Tabs, Merge
    // All Windows, Move Tab to New Window).
    let windowItem = NSMenuItem()
    mainMenu.addItem(windowItem)
    let windowMenu = NSMenu(title: "Window")
    windowItem.submenu = windowMenu
    windowMenu.addItem(withTitle: "Minimize", action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m")
    windowMenu.addItem(withTitle: "Zoom", action: #selector(NSWindow.performZoom(_:)), keyEquivalent: "")
    NSApplication.shared.windowsMenu = windowMenu

    return mainMenu
}

let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = AppDelegate()
app.delegate = delegate
app.mainMenu = makeMenu(target: delegate)
app.run()
