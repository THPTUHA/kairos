export function formatDate(t: number) {
    let d;
    if (!t) {
        d = new Date()
    } else {
        d = new Date(t * 1000)
    }
    return d.toLocaleString()
}