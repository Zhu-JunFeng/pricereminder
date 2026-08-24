using System.Runtime.InteropServices;

namespace PriceReminder.Windows;

internal static class BrandIcon
{
    public static Icon Create(int size = 64)
    {
        using var bitmap = new Bitmap(size, size, System.Drawing.Imaging.PixelFormat.Format32bppArgb);
        using var graphics = Graphics.FromImage(bitmap);
        graphics.SmoothingMode = System.Drawing.Drawing2D.SmoothingMode.AntiAlias;
        graphics.Clear(Color.FromArgb(8, 126, 134));
        using var white = new SolidBrush(Color.FromArgb(247, 252, 251));
        using var teal = new Pen(Color.FromArgb(8, 126, 134), Math.Max(3, size / 13f))
        {
            StartCap = System.Drawing.Drawing2D.LineCap.Round,
            EndCap = System.Drawing.Drawing2D.LineCap.Round,
            LineJoin = System.Drawing.Drawing2D.LineJoin.Round,
        };
        graphics.FillEllipse(white, size * .2f, size * .16f, size * .6f, size * .6f);
        graphics.FillRectangle(white, size * .2f, size * .48f, size * .6f, size * .22f);
        graphics.FillEllipse(white, size * .13f, size * .57f, size * .74f, size * .27f);
        graphics.DrawLines(teal,
        [
            new PointF(size * .28f, size * .59f),
            new PointF(size * .43f, size * .43f),
            new PointF(size * .55f, size * .54f),
            new PointF(size * .73f, size * .34f),
        ]);
        var handle = bitmap.GetHicon();
        try { return (Icon)Icon.FromHandle(handle).Clone(); }
        finally { DestroyIcon(handle); }
    }

    [DllImport("user32.dll")]
    private static extern bool DestroyIcon(IntPtr handle);
}
