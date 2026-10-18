// Highlighter desktop app: runs the highlighter server (a Go sidecar bundled
// next to this executable, with yt-dlp, ffmpeg and deno) and shows its web UI
// in a native window. Closing the window stops the server; clicking Quit in
// the UI stops the server, which closes the app.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::fs::{self, File};
use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant};

use tauri::webview::NewWindowResponse;
use tauri::{AppHandle, Manager, RunEvent, Url, WebviewUrl, WebviewWindow, WebviewWindowBuilder};
use tauri_plugin_opener::OpenerExt;

/// The server's usual port; another free port is used when it is taken.
/// HIGHLIGHTER_PORT overrides it (smoke tests).
const PREFERRED_PORT: u16 = 7777;
const STARTUP_TIMEOUT: Duration = Duration::from_secs(90);

/// The running server process, killed when the app exits.
struct Server(Mutex<Option<Child>>);

fn main() {
    let app = tauri::Builder::default()
        // A second launch focuses the existing window instead of starting another server.
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.unminimize();
                let _ = window.show();
                let _ = window.set_focus();
            }
        }))
        .plugin(tauri_plugin_opener::init())
        .manage(Server(Mutex::new(None)))
        .setup(|app| {
            let window = build_window(app.handle())?;
            start_server(app.handle().clone(), window);
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("failed to build the Tauri app");

    app.run(|app, event| {
        if let RunEvent::Exit = event {
            if let Some(mut child) = app.state::<Server>().0.lock().unwrap().take() {
                let _ = child.kill();
                let _ = child.wait();
            }
        }
    });
}

/// The main window starts on the bundled splash page and is pointed at the
/// server once it answers. Links to other sites open in the default browser.
fn build_window(app: &AppHandle) -> tauri::Result<WebviewWindow> {
    let nav_app = app.clone();
    let popup_app = app.clone();
    WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
        .title("Highlighter")
        .inner_size(1280.0, 860.0)
        .min_inner_size(820.0, 560.0)
        .center()
        .on_navigation(move |url| {
            if is_app_url(url) {
                return true;
            }
            let _ = nav_app.opener().open_url(url.as_str(), None::<&str>);
            false
        })
        .on_new_window(move |url, _features| {
            let _ = popup_app.opener().open_url(url.as_str(), None::<&str>);
            NewWindowResponse::Deny
        })
        .build()
}

/// The splash page (tauri://localhost, or http://tauri.localhost on Windows)
/// and the local server. Embedded frames (the YouTube player) are not
/// main-frame navigations and are unaffected.
fn is_app_url(url: &Url) -> bool {
    matches!(url.scheme(), "tauri" | "about" | "data" | "blob")
        || matches!(url.host_str(), Some("127.0.0.1" | "localhost" | "tauri.localhost"))
}

fn start_server(app: AppHandle, window: WebviewWindow) {
    let log_path = log_file(&app);
    let port = choose_port();
    let child = match spawn_server(port, &log_path) {
        Ok(child) => child,
        Err(err) => return show_error(&window, &format!("The server could not be started ({err})."), &log_path),
    };
    *app.state::<Server>().0.lock().unwrap() = Some(child);

    let ready = Arc::new(AtomicBool::new(false));
    watch_server(app.clone(), window.clone(), ready.clone(), log_path.clone());

    thread::spawn(move || {
        let deadline = Instant::now() + STARTUP_TIMEOUT;
        while Instant::now() < deadline {
            if app.state::<Server>().0.lock().unwrap().is_none() {
                return; // exited; watch_server reports it
            }
            if ping(port) {
                ready.store(true, Ordering::SeqCst);
                let url = Url::parse(&format!("http://127.0.0.1:{port}/")).expect("valid URL");
                let _ = window.navigate(url);
                return;
            }
            thread::sleep(Duration::from_millis(250));
        }
        show_error(&window, "The server did not respond in time.", &log_path);
    });
}

/// Polls the server process: if it stops after the UI is up (Quit was
/// clicked) the app exits; if it stops during startup the splash shows why.
fn watch_server(app: AppHandle, window: WebviewWindow, ready: Arc<AtomicBool>, log_path: PathBuf) {
    thread::spawn(move || loop {
        thread::sleep(Duration::from_millis(300));
        let state = app.state::<Server>();
        let mut guard = state.0.lock().unwrap();
        let Some(child) = guard.as_mut() else { return };
        match child.try_wait() {
            Ok(None) => continue,
            Ok(Some(status)) => {
                guard.take();
                drop(guard);
                if ready.load(Ordering::SeqCst) {
                    app.exit(0);
                } else {
                    show_error(&window, &format!("The server stopped while starting ({status})."), &log_path);
                }
                return;
            }
            Err(_) => return,
        }
    });
}

fn spawn_server(port: u16, log_path: &Path) -> std::io::Result<Child> {
    let exe = std::env::current_exe()?;
    let dir = exe.parent().expect("executable has a folder");
    let server = dir.join(if cfg!(windows) { "highlighter-server.exe" } else { "highlighter-server" });
    let log = File::create(log_path)?;
    let mut cmd = Command::new(server);
    cmd.args(["-no-open", "-port", &port.to_string()])
        .stdin(Stdio::null())
        .stdout(log.try_clone()?)
        .stderr(log);
    #[cfg(windows)]
    {
        // No console window for the server; its yt-dlp/ffmpeg children share
        // its hidden console, so they do not flash windows either.
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        cmd.creation_flags(CREATE_NO_WINDOW);
    }
    cmd.spawn()
}

fn choose_port() -> u16 {
    if let Some(port) = std::env::var("HIGHLIGHTER_PORT").ok().and_then(|p| p.parse().ok()) {
        return port;
    }
    if TcpListener::bind(("127.0.0.1", PREFERRED_PORT)).is_ok() {
        return PREFERRED_PORT;
    }
    TcpListener::bind(("127.0.0.1", 0))
        .and_then(|l| l.local_addr())
        .map(|a| a.port())
        .unwrap_or(PREFERRED_PORT)
}

/// Whether a highlighter server answers on port (GET /api/ping).
fn ping(port: u16) -> bool {
    let addr = SocketAddr::from(([127, 0, 0, 1], port));
    let Ok(mut stream) = TcpStream::connect_timeout(&addr, Duration::from_millis(500)) else {
        return false;
    };
    let _ = stream.set_read_timeout(Some(Duration::from_secs(2)));
    let request = format!("GET /api/ping HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nConnection: close\r\n\r\n");
    if stream.write_all(request.as_bytes()).is_err() {
        return false;
    }
    let mut response = String::new();
    let _ = stream.read_to_string(&mut response);
    response.starts_with("HTTP/1.1 200") && response.contains("\"highlighter\"")
}

fn log_file(app: &AppHandle) -> PathBuf {
    let dir = app.path().app_log_dir().unwrap_or_else(|_| std::env::temp_dir());
    let _ = fs::create_dir_all(&dir);
    dir.join("server.log")
}

fn show_error(window: &WebviewWindow, message: &str, log_path: &Path) {
    let js = format!(
        "window.showError({}, {})",
        serde_json::to_string(message).unwrap(),
        serde_json::to_string(&log_path.display().to_string()).unwrap()
    );
    let _ = window.eval(&js);
}
