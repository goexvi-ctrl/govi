import Cocoa

// Govi: a native macOS application with the govi (Go nvi) editor engine embedded
// in-process. The engine is linked in as a C archive (libgovi); this app drives
// it and renders its screen in a custom NSView. nvi is *embedded*, not exec'd.
//
// The app is multi-window: each EditorWindow owns its own engine instance
// (a libgovi handle), so File > New and File > Open each spawn an independent
// editor.

// EditorWindow owns one window, its GoviView, and the embedded engine behind it.
// Instances keep themselves alive in a static set until their window closes.
final class EditorWindow: NSObject, NSWindowDelegate {
    private static var windows: Set<EditorWindow> = []
    private static var cascadePoint = NSPoint.zero

    let window: NSWindow
    let view: GoviView

    // make creates an editor for path (empty path = an empty buffer) without
    // presenting it. Returns nil if the file could not be opened.
    private static func make(path: String) -> EditorWindow? {
        let handle = path.withCString { GoviStart(UnsafeMutablePointer(mutating: $0)) }
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

    // openTab presents path as a new tab in keyWindow's tab group, or as a
    // standalone window if there is no key window to tab into.
    static func openTab(in keyWindow: NSWindow?, path: String) {
        guard let w = make(path: path) else { return }
        if let key = keyWindow, key !== w.window {
            key.addTabbedWindow(w.window, ordered: .above)
            w.window.makeKeyAndOrderFront(nil)
            w.window.makeFirstResponder(w.view)
            w.view.updateGeometry()
        } else {
            w.showStandalone()
        }
    }

    private init(handle: Int64, path: String) {
        let frame = NSRect(x: 0, y: 0, width: 800, height: 600)
        window = NSWindow(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)
        view = GoviView(frame: frame, handle: handle)
        super.init()
        window.title = path.isEmpty ? "Untitled" : (path as NSString).lastPathComponent
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
    }

    func windowWillClose(_ notification: Notification) {
        GoviClose(view.handle)
        EditorWindow.windows.remove(self)
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        let args = CommandLine.arguments
        let path = args.count > 1 ? args[1] : ""
        EditorWindow.open(path: path)
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

    // File > Open…: choose one or more files, each in its own window.
    @objc func openWindow(_ sender: Any?) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = true
        panel.begin { response in
            guard response == .OK else { return }
            for url in panel.urls {
                EditorWindow.open(path: url.path)
            }
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
