Add-Type -AssemblyName System.Windows.Forms
$notify = New-Object Windows.Forms.NotifyIcon
$notify.Icon = [Drawing.SystemIcons]::Application
$notify.Text = 'Portivane'
$notify.Visible = $true
$menu = New-Object Windows.Forms.ContextMenuStrip
$open = $menu.Items.Add('Open Portivane')
$open.Add_Click({ Start-Process 'http://127.0.0.1:4747' })
$exit = $menu.Items.Add('Exit')
$exit.Add_Click({ $notify.Visible = $false; [Windows.Forms.Application]::Exit() })
$notify.ContextMenuStrip = $menu
[Windows.Forms.Application]::Run()
