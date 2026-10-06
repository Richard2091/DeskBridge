use std::sync::Mutex;
use tauri::{Manager, RunEvent};
use tauri_plugin_shell::{process::{CommandChild, CommandEvent}, ShellExt};

/// 保存 sidecar 子进程句柄，退出时结束它。
struct Sidecar(Mutex<Option<CommandChild>>);

#[tauri::command]
fn autostream() -> bool {
    std::env::var("DESKBRIDGE_AUTOSTREAM").map(|v| v == "1").unwrap_or(false)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let app = tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .manage(Sidecar(Mutex::new(None)))
        .setup(|app| {
            let (mut rx, child) = app
                .shell()
                .sidecar("uicore-server")?
                .args(["-addr", "127.0.0.1:47800"])
                .spawn()?;
            *app.state::<Sidecar>().0.lock().unwrap() = Some(child);
            tauri::async_runtime::spawn(async move {
                while let Some(ev) = rx.recv().await {
                    match ev {
                        CommandEvent::Stdout(l) => println!("[core] {}", String::from_utf8_lossy(&l)),
                        CommandEvent::Stderr(l) => eprintln!("[core] {}", String::from_utf8_lossy(&l)),
                        CommandEvent::Terminated(t) => {
                            println!("[core] terminated: {:?}", t);
                            break;
                        }
                        _ => {}
                    }
                }
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![autostream])
        .build(tauri::generate_context!())
        .expect("error while building tauri application");

    app.run(|handle, event| {
        if let RunEvent::Exit = event {
            if let Some(child) = handle.state::<Sidecar>().0.lock().unwrap().take() {
                let _ = child.kill();
            }
        }
    });
}
