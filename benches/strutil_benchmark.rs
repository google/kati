use criterion::{Criterion, black_box, criterion_group, criterion_main};
use kati::strutil::word_scanner;

fn criterion_benchmark(c: &mut Criterion) {
    let word = "frameworks/base/docs/html/tv/adt-1/index.jd ";
    let s = word.repeat(400000 / word.len());

    c.bench_function("wordscanner", |b| {
        b.iter(|| black_box(word_scanner(black_box(s.as_bytes())).collect::<Vec<&[u8]>>()))
    });
}

criterion_group!(benches, criterion_benchmark);
criterion_main!(benches);
