use std::process::ExitCode;

use deskbridge_desktop_helper::version_line;

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    match args.first().map(String::as_str) {
        Some("--version") => {
            println!("{}", version_line());
            ExitCode::SUCCESS
        }
        _ => {
            eprintln!("用法：deskbridge-desktop-helper --version（协议尚未实现）");
            ExitCode::from(2)
        }
    }
}
