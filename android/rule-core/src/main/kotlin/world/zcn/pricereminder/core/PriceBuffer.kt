package world.zcn.pricereminder.core

class PriceBuffer {
    private val pointsBySymbol = mutableMapOf<String, MutableList<PricePoint>>()

    @Synchronized
    fun add(point: PricePoint): Boolean {
        val points = pointsBySymbol.getOrPut(point.symbol) { mutableListOf() }
        points.lastOrNull()?.let { last ->
            if (point.eventTime < last.eventTime) return false
            if (point.eventTime / 1_000 == last.eventTime / 1_000) {
                points[points.lastIndex] = point
                return true
            }
        }
        points += point
        val cutoff = point.eventTime - RETENTION_MILLIS
        while (points.isNotEmpty() && points.first().eventTime < cutoff) points.removeAt(0)
        return true
    }

    @Synchronized
    fun atOrBefore(symbol: String, eventTime: Long): PricePoint? {
        val points = pointsBySymbol[symbol] ?: return null
        var low = 0
        var high = points.size
        while (low < high) {
            val middle = (low + high) ushr 1
            if (points[middle].eventTime <= eventTime) low = middle + 1 else high = middle
        }
        return if (low == 0) null else points[low - 1]
    }

    @Synchronized
    fun latest(symbol: String): PricePoint? = pointsBySymbol[symbol]?.lastOrNull()

    @Synchronized
    fun covers(symbol: String, durationMillis: Long): Boolean {
        val points = pointsBySymbol[symbol] ?: return false
        return points.size >= 2 && points.last().eventTime - points.first().eventTime >= durationMillis
    }

    @Synchronized
    fun points(symbol: String): List<PricePoint> = pointsBySymbol[symbol]?.toList() ?: emptyList()

    @Synchronized
    fun symbols(): Set<String> = pointsBySymbol.keys.toSet()

    @Synchronized
    fun restore(points: Iterable<PricePoint>) {
        points.sortedWith(compareBy<PricePoint> { it.eventTime }.thenBy { it.symbol }).forEach(::add)
    }

    companion object { const val RETENTION_MILLIS = 60L * 60L * 1_000L }
}
