#[macro_export]
macro_rules! handle {
    ($value:expr, $err:ident, $onerr:expr) => {
        match $value {
            Ok(ok) => ok,
            Err($err) => $onerr,
        }
    };
}
