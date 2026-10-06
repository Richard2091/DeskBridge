@echo off
set LOG=D:\data\AI\Projects\deskbridge\spikes\ui-flutter\build.log
echo START %DATE% %TIME% > %LOG%
call D:\data\AI\Projects\deskbridge\spikes\ui-flutter\build.cmd >> %LOG% 2>&1
echo END %DATE% %TIME% >> %LOG%