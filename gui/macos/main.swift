import Cocoa

// Govi: a native macOS application with the govi (Go nvi) editor engine embedded
// in-process. The engine is linked in as a C archive (libgovi); this app drives
// it and renders its screen in a custom NSView. nvi is *embedded*, not exec'd.

final class AppDelegate: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    var view: GoviView!

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Start the engine, opening a file argument if one was given.
        let args = CommandLine.arguments
        let path = args.count > 1 ? args[1] : ""
        _ = path.withCString { GoviStart(UnsafeMutablePointer(mutating: $0)) }

        let frame = NSRect(x: 0, y: 0, width: 800, height: 600)
        window = NSWindow(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered,
            defer: false)
        window.title = path.isEmpty ? "govi" : path
        window.center()

        view = GoviView(frame: frame)
        view.autoresizingMask = [.width, .height]
        window.contentView = view

        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(view)
        view.updateGeometry()

        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }
}

// Build a minimal main menu so the standard shortcuts (Cmd-Q) work. Editing
// commands all go through the engine, not the menu.
func makeMenu() -> NSMenu {
    let mainMenu = NSMenu()
    let appItem = NSMenuItem()
    mainMenu.addItem(appItem)
    let appMenu = NSMenu()
    appItem.submenu = appMenu
    let name = ProcessInfo.processInfo.processName
    appMenu.addItem(withTitle: "About \(name)", action: nil, keyEquivalent: "")
    appMenu.addItem(NSMenuItem.separator())
    appMenu.addItem(withTitle: "Quit \(name)",
                    action: #selector(NSApplication.terminate(_:)),
                    keyEquivalent: "q")

    // Edit menu: Cut/Copy/Paste/Select All route through the responder chain to
    // GoviView's overrides, so the standard Cmd-X/C/V/A shortcuts work.
    let editItem = NSMenuItem()
    mainMenu.addItem(editItem)
    let editMenu = NSMenu(title: "Edit")
    editItem.submenu = editMenu
    editMenu.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
    editMenu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
    editMenu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
    editMenu.addItem(NSMenuItem.separator())
    editMenu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
    return mainMenu
}

let app = NSApplication.shared
app.setActivationPolicy(.regular)
app.mainMenu = makeMenu()
let delegate = AppDelegate()
app.delegate = delegate
app.run()
