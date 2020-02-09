#[macro_export]
macro_rules! handle {
    ($value:expr, $err:ident, $on_err:expr) => {
        match $value {
            Ok(ok) => ok,
            Err($err) => $on_err,
        }
    };
}
