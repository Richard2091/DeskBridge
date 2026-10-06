// 技术验证：C# + FlaUI 操作记事本。步骤见 spikes/README.md。
using System.Diagnostics;
using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.Encodings.Web;
using System.Text.Json;
using FlaUI.Core;
using FlaUI.Core.AutomationElements;
using FlaUI.Core.Definitions;
using FlaUI.Core.Input;
using FlaUI.Core.WindowsAPI;
using FlaUI.UIA3;

const string Lang = "cs";
const string TestText = "你好，DeskBridge 测试 123";

var res = new Dictionary<string, object?>();
var sw = Stopwatch.StartNew();

res["dpi_aware"] = Native.SetProcessDpiAwarenessContext(new IntPtr(-4)); // PER_MONITOR_AWARE_V2
res["screen_physical"] = new[] { Native.GetSystemMetrics(0), Native.GetSystemMetrics(1) };

var tmp = Path.GetTempPath();
var txt = Path.Combine(tmp, $"deskbridge-spike-{Lang}.txt");
var png = Path.Combine(tmp, $"deskbridge-spike-{Lang}.png");
File.WriteAllText(txt, "");

using var automation = new UIA3Automation();
var app = Application.Launch("notepad.exe", $"\"{txt}\"");
try
{
    var win = app.GetMainWindow(automation, TimeSpan.FromSeconds(10));
    res["window_title"] = win.Title;
    res["window_rect"] = Rect(win.BoundingRectangle);

    // 3. 元素树
    var sample = new List<string>();
    res["tree_nodes"] = Walk(win, 0, sample);
    res["tree_sample"] = sample;

    // 4. 按名称展开“文件”菜单
    var fileMenu = win.FindAllDescendants(cf => cf.ByControlType(ControlType.MenuItem))
        .FirstOrDefault(e => e.Name.StartsWith("文件") || e.Name.StartsWith("File"));
    res["menu_item"] = fileMenu?.Name;
    if (fileMenu != null)
    {
        fileMenu.AsMenuItem().Expand();
        Thread.Sleep(500);
        res["menu_by_pattern"] = MenuEntries(automation);
        Keyboard.Press(VirtualKeyShort.ESCAPE);
        Thread.Sleep(300);

        // 5. 按坐标点击
        var r = fileMenu.BoundingRectangle;
        res["menu_item_rect"] = Rect(r);
        Mouse.Click(new Point(r.X + r.Width / 2, r.Y + r.Height / 2));
        Thread.Sleep(500);
        res["menu_by_coordinates"] = MenuEntries(automation);
        Keyboard.Press(VirtualKeyShort.ESCAPE);
        Thread.Sleep(300);
    }

    // 6. 输入中文并保存
    var edit = win.FindFirstDescendant(cf => cf.ByControlType(ControlType.Document))
               ?? win.FindFirstDescendant(cf => cf.ByControlType(ControlType.Edit));
    if (edit == null) throw new Exception("找不到编辑区");
    edit.Focus();
    Keyboard.Type(TestText);
    Thread.Sleep(300);
    res["edit_value_via_uia"] = edit.Patterns.Value.PatternOrDefault?.Value.Value;
    Keyboard.TypeSimultaneously(VirtualKeyShort.CONTROL, VirtualKeyShort.KEY_S);
    Thread.Sleep(800);
    var saved = File.ReadAllText(txt, Encoding.UTF8).TrimStart('﻿');
    res["saved_text"] = saved;
    res["typed_ok"] = saved == TestText;

    // 7. 截图
    int w = Native.GetSystemMetrics(0), h = Native.GetSystemMetrics(1);
    using (var bmp = new Bitmap(w, h))
    {
        using (var g = Graphics.FromImage(bmp)) g.CopyFromScreen(0, 0, 0, 0, new Size(w, h));
        bmp.Save(png, ImageFormat.Png);
    }
    res["screenshot"] = new { path = png, width = w, height = h, bytes = new FileInfo(png).Length };
}
catch (Exception e)
{
    res["error"] = e.ToString();
}
finally
{
    try { app.Close(); } catch { }
    Thread.Sleep(300);
    try { if (!app.HasExited) app.Kill(); } catch { }
}

res["elapsed_ms"] = sw.ElapsedMilliseconds;
Console.OutputEncoding = Encoding.UTF8;
Console.WriteLine(JsonSerializer.Serialize(res, new JsonSerializerOptions
{
    WriteIndented = true,
    Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
}));

static int Walk(AutomationElement e, int depth, List<string> sample)
{
    string ct;
    try { ct = e.ControlType.ToString(); } catch { ct = "?"; }
    if (sample.Count < 15) sample.Add($"{new string(' ', depth * 2)}{ct} '{e.Name}'");
    int n = 1;
    if (depth >= 6) return n;
    foreach (var c in e.FindAllChildren()) n += Walk(c, depth + 1, sample);
    return n;
}

// 弹出菜单是桌面的直接子元素，读取前几项名称作为“菜单已打开”的证据
static string[]? MenuEntries(UIA3Automation automation)
{
    var menu = automation.GetDesktop().FindFirstChild(cf => cf.ByControlType(ControlType.Menu));
    return menu?.FindAllChildren().Take(5).Select(c => c.Name).ToArray();
}

static int[] Rect(Rectangle r) => [r.X, r.Y, r.Width, r.Height];

static class Native
{
    [DllImport("user32.dll")] public static extern bool SetProcessDpiAwarenessContext(IntPtr value);
    [DllImport("user32.dll")] public static extern int GetSystemMetrics(int index);
}
