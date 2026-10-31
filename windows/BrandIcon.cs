namespace PriceReminder.Windows;

internal static class BrandIcon
{
    public static Icon Create(int size = 64)
    {
        using var stream = typeof(BrandIcon).Assembly.GetManifestResourceStream("PriceReminder.Brand.ico")
            ?? throw new InvalidOperationException("PriceReminder icon resource is missing.");
        using var icon = new Icon(stream, size, size);
        return (Icon)icon.Clone();
    }
}
