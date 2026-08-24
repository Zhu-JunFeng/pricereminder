using Microsoft.Win32;

namespace PriceReminder.Windows;

internal static class StartupManager
{
    private const string RegistryPath = @"Software\Microsoft\Windows\CurrentVersion\Run";
    private const string ValueName = "PriceReminder";

    public static bool Enabled
    {
        get
        {
            using var key = Registry.CurrentUser.OpenSubKey(RegistryPath);
            return key?.GetValue(ValueName) is string;
        }
        set
        {
            using var key = Registry.CurrentUser.CreateSubKey(RegistryPath);
            if (value)
                key.SetValue(ValueName, $"\"{Application.ExecutablePath}\" --tray");
            else
                key.DeleteValue(ValueName, false);
        }
    }
}
