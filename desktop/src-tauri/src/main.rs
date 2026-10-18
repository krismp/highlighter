// Highlighter desktop app: runs the highlighter server (a Go sidecar bundled
// next to this executable, with yt-dlp, ffmpeg and deno) and shows its web UI
// in a native window.
//
// Nothing outlives the app. On exit the server is told to shut down (it stops
// every yt-dlp/ffmpeg run with its child processes and removes partial files)
// and is killed if it takes too long. If the app dies without that, the
// server notices its stdin closing and shuts down anyway, and on Windows a
// kill-on-close job object ends the whole process tree with the app.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::fs::{self, File};
use std::io::{Read, Write};
use std::net::{SocketAddr, TcpListener, TcpStream};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicBool, AtomicU16, Ordering};
use std::sync::Mutex;
use std::thread;
use std::time::{Duration, Instant};

use tauri::menu::{Menu, MenuItem, PredefinedMenuItem, Submenu};
use tauri::webview::NewWindowResponse;
use tauri::{AppHandle, Manager, RunEvent, Url, WebviewUrl, WebviewWindow, WebviewWindowBuilder, WindowEvent, Wry};
use tauri_plugin_dialog::{DialogExt, MessageDialogButtons, MessageDialogKind};
use tauri_plugin_opener::OpenerExt;

/// The server's usual port; another free port is used when it is taken.
/// HIGHLIGHTER_PORT overrides it (smoke tests).
const PREFERRED_PORT: u16 = 7777;
const STARTUP_TIMEOUT: Duration = Duration::from_secs(90);
/// How long the server gets to stop its clip work before it is killed.
const SHUTDOWN_TIMEOUT: Duration = Duration::from_secs(15);

#[derive(Default)]
struct Server {
    child: Mutex<Option<Child>>,
    /// Held open while the app runs; closing it tells the server to shut down.
    stdin: Mutex<Option<ChildStdin>>,
    port: AtomicU16,
    /// Set once the UI is showing: from then on the server stopping means Quit was clicked.
    ready: AtomicBool,
    stopping: AtomicBool,
}

/// The user agreed to quit (or nothing was processing), so closing proceeds without asking.
static QUIT_CONFIRMED: AtomicBool = AtomicBool::new(false);
/// A "videos are still processing" dialog is open.
static ASKING: AtomicBool = AtomicBool::new(false);

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
        .plugin(tauri_plugin_dialog::init())
        .manage(Server::default())
        .menu(app_menu)
        .on_menu_event(|app, event| {
            if event.id() == QUIT_MENU_ID && may_quit(app) {
                app.exit(0);
            }
        })
        .setup(|app| {
            let window = build_window(app.handle())?;
            start_server(app.handle().clone(), window);
            Ok(())
        })
        .on_window_event(|window, event| {
            if let WindowEvent::CloseRequested { api, .. } = event {
                if !may_quit(window.app_handle()) {
                    api.prevent_close();
                }
            }
        })
        .build(tauri::generate_context!())
        .expect("failed to build the Tauri app");

    app.run(|app, event| match event {
        // Cmd+Q, the app menu's Quit, or the last window closing.
        RunEvent::ExitRequested { api, .. } => {
            if !may_quit(app) {
                api.prevent_exit();
            }
        }
        RunEvent::Exit => stop_server(app),
        _ => {}
    });
}

const QUIT_MENU_ID: &str = "quit";

/// The app's menu. On macOS the standard Quit item (and Cmd+Q) terminates
/// the app without the exit hook that asks about running videos, so Quit is
/// our own item. The Edit menu keeps copy and paste working in text fields.
/// Windows gets no menu bar, as before.
fn app_menu(app: &AppHandle) -> tauri::Result<Menu<Wry>> {
    if !cfg!(target_os = "macos") {
        return Menu::new(app);
    }
    let quit = MenuItem::with_id(app, QUIT_MENU_ID, "Quit Highlighter", true, Some("CmdOrCtrl+Q"))?;
    let sep = || PredefinedMenuItem::separator(app);
    let app_items = Submenu::with_items(app, "Highlighter", true, &[
        &PredefinedMenuItem::about(app, None, None)?,
        &sep()?,
        &PredefinedMenuItem::hide(app, None)?,
        &PredefinedMenuItem::hide_others(app, None)?,
        &PredefinedMenuItem::show_all(app, None)?,
        &sep()?,
        &quit,
    ])?;
    let edit = Submenu::with_items(app, "Edit", true, &[
        &PredefinedMenuItem::undo(app, None)?,
        &PredefinedMenuItem::redo(app, None)?,
        &sep()?,
        &PredefinedMenuItem::cut(app, None)?,
        &PredefinedMenuItem::copy(app, None)?,
        &PredefinedMenuItem::paste(app, None)?,
        &PredefinedMenuItem::select_all(app, None)?,
    ])?;
    let window = Submenu::with_items(app, "Window", true, &[
        &PredefinedMenuItem::minimize(app, None)?,
        &PredefinedMenuItem::maximize(app, None)?,
        &sep()?,
        &PredefinedMenuItem::close_window(app, None)?,
    ])?;
    Menu::with_items(app, &[&app_items, &edit, &window])
}

/// Whether the app may close now. If videos are still being processed it
/// asks first (and returns false); choosing "Quit Anyway" exits the app.
fn may_quit(app: &AppHandle) -> bool {
    if QUIT_CONFIRMED.load(Ordering::SeqCst) {
        return true;
    }
    let port = app.state::<Server>().port.load(Ordering::SeqCst);
    let titles = if port == 0 { Vec::new() } else { running_videos(port) };
    if titles.is_empty() {
        QUIT_CONFIRMED.store(true, Ordering::SeqCst);
        return true;
    }
    if ASKING.swap(true, Ordering::SeqCst) {
        return false; // the question is already on screen
    }
    let list: String = titles.iter().map(|t| format!("\n• {t}")).collect();
    let message = format!(
        "{} still being processed:{list}\n\nQuitting now stops processing. Clips that are already done stay saved; unfinished ones are not saved.",
        if titles.len() == 1 { "A video is".to_string() } else { format!("{} videos are", titles.len()) }
    );
    let handle = app.clone();
    let mut dialog = app
        .dialog()
        .message(message)
        .title("Quit Highlighter?")
        .kind(MessageDialogKind::Warning)
        .buttons(MessageDialogButtons::OkCancelCustom("Quit Anyway".into(), "Keep Processing".into()));
    if let Some(window) = app.get_webview_window("main") {
        dialog = dialog.parent(&window);
    }
    dialog.show(move |quit| {
        ASKING.store(false, Ordering::SeqCst);
        if quit {
            QUIT_CONFIRMED.store(true, Ordering::SeqCst);
            handle.exit(0);
        }
    });
    false
}

/// The main window starts on the bundled splash page and is pointed at the
/// server once it answers. Links that open a new window (the YouTube
/// player's "Watch on YouTube", for one) go to the default browser.
///
/// There is deliberately no navigation filter: on macOS it also sees every
/// embedded frame's navigation (wry does not say which frame), so filtering
/// out other sites cancelled the YouTube player and opened it in the browser
/// instead, where it shows "Error 153". The app's own pages link nowhere else.
fn build_window(app: &AppHandle) -> tauri::Result<WebviewWindow> {
    let popup_app = app.clone();
    WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
        .title("Highlighter")
        .inner_size(1280.0, 860.0)
        .min_inner_size(820.0, 560.0)
        .center()
        .on_new_window(move |url, _features| {
            let _ = popup_app.opener().open_url(url.as_str(), None::<&str>);
            NewWindowResponse::Deny
        })
        .build()
}

fn start_server(app: AppHandle, window: WebviewWindow) {
    let log_path = log_file(&app);
    let port = choose_port();
    let (child, stdin) = match spawn_server(port, &log_path) {
        Ok(started) => started,
        Err(err) => return show_error(&window, &format!("The server could not be started ({err})."), &log_path),
    };
    let server = app.state::<Server>();
    server.port.store(port, Ordering::SeqCst);
    *server.stdin.lock().unwrap() = Some(stdin);
    *server.child.lock().unwrap() = Some(child);

    watch_server(app.clone(), window.clone(), log_path.clone());
    thread::spawn(move || {
        let deadline = Instant::now() + STARTUP_TIMEOUT;
        while Instant::now() < deadline {
            if app.state::<Server>().child.lock().unwrap().is_none() {
                return; // exited; watch_server reports it
            }
            if http_get(port, "/api/ping").is_some_and(|body| body.contains("\"highlighter\"")) {
                app.state::<Server>().ready.store(true, Ordering::SeqCst);
                let url = Url::parse(&format!("http://127.0.0.1:{port}{}", start_path())).expect("valid URL");
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
fn watch_server(app: AppHandle, window: WebviewWindow, log_path: PathBuf) {
    thread::spawn(move || loop {
        thread::sleep(Duration::from_millis(300));
        let server = app.state::<Server>();
        if server.stopping.load(Ordering::SeqCst) {
            return; // stop_server owns the process now
        }
        let mut guard = server.child.lock().unwrap();
        let Some(child) = guard.as_mut() else { return };
        match child.try_wait() {
            Ok(None) => continue,
            Ok(Some(status)) => {
                guard.take();
                drop(guard);
                if server.ready.load(Ordering::SeqCst) {
                    QUIT_CONFIRMED.store(true, Ordering::SeqCst); // confirmed in the UI already
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

/// Asks the server to shut down by closing its stdin, waits for it to stop
/// its clip work, and kills it if it takes longer than SHUTDOWN_TIMEOUT.
fn stop_server(app: &AppHandle) {
    let server = app.state::<Server>();
    server.stopping.store(true, Ordering::SeqCst);
    drop(server.stdin.lock().unwrap().take());
    let Some(mut child) = server.child.lock().unwrap().take() else { return };
    let deadline = Instant::now() + SHUTDOWN_TIMEOUT;
    while Instant::now() < deadline {
        if !matches!(child.try_wait(), Ok(None)) {
            return;
        }
        thread::sleep(Duration::from_millis(100));
    }
    let _ = child.kill();
    let _ = child.wait();
}

fn spawn_server(port: u16, log_path: &Path) -> std::io::Result<(Child, ChildStdin)> {
    let exe = std::env::current_exe()?;
    let dir = exe.parent().expect("executable has a folder");
    let server = dir.join(if cfg!(windows) { "highlighter-server.exe" } else { "highlighter-server" });
    let log = File::create(log_path)?;
    let mut cmd = Command::new(server);
    cmd.args(["-no-open", "-exit-with-stdin", "-port", &port.to_string()])
        .stdin(Stdio::piped())
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
    let mut child = cmd.spawn()?;
    #[cfg(windows)]
    end_with_app(&child);
    let stdin = child.stdin.take().expect("stdin is piped");
    Ok((child, stdin))
}

/// Puts the server in a job object that Windows kills, with every process
/// the server started, when this app's last handle to it closes: at exit,
/// crash or force-quit alike.
#[cfg(windows)]
fn end_with_app(child: &Child) {
    use std::os::windows::io::AsRawHandle;
    use windows_sys::Win32::System::JobObjects::{
        AssignProcessToJobObject, CreateJobObjectW, JobObjectExtendedLimitInformation, SetInformationJobObject,
        JOBOBJECT_EXTENDED_LIMIT_INFORMATION, JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    };
    unsafe {
        let job = CreateJobObjectW(std::ptr::null(), std::ptr::null());
        if job.is_null() {
            return;
        }
        let mut info: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();
        info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
        SetInformationJobObject(
            job,
            JobObjectExtendedLimitInformation,
            &info as *const _ as *const std::ffi::c_void,
            std::mem::size_of_val(&info) as u32,
        );
        AssignProcessToJobObject(job, child.as_raw_handle() as _);
        // The job handle is deliberately never closed: Windows closes it when
        // this process ends, which is what triggers the kill.
    }
}

/// The page the window opens on: "/", or HIGHLIGHTER_START_PATH (smoke tests).
fn start_path() -> String {
    std::env::var("HIGHLIGHTER_START_PATH")
        .ok()
        .filter(|p| p.starts_with('/') && !p.starts_with("//"))
        .unwrap_or_else(|| "/".into())
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

/// GETs path from the local server; the body of a 200 response, else None.
fn http_get(port: u16, path: &str) -> Option<String> {
    let addr = SocketAddr::from(([127, 0, 0, 1], port));
    let mut stream = TcpStream::connect_timeout(&addr, Duration::from_millis(500)).ok()?;
    stream.set_read_timeout(Some(Duration::from_secs(2))).ok()?;
    let request = format!("GET {path} HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\nConnection: close\r\n\r\n");
    stream.write_all(request.as_bytes()).ok()?;
    let mut response = String::new();
    let _ = stream.read_to_string(&mut response);
    if !response.starts_with("HTTP/1.1 200") {
        return None;
    }
    response.split_once("\r\n\r\n").map(|(_, body)| body.to_string())
}

/// Titles of the videos the server is still processing; empty when none or
/// when the server cannot be asked.
fn running_videos(port: u16) -> Vec<String> {
    let Some(body) = http_get(port, "/api/jobs/running") else { return Vec::new() };
    let Ok(value) = serde_json::from_str::<serde_json::Value>(&body) else { return Vec::new() };
    value["running"]
        .as_array()
        .map(|items| items.iter().filter_map(|i| i["title"].as_str().map(str::to_string)).collect())
        .unwrap_or_default()
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
