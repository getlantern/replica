 -- Query for https://github.com/getlantern/grants/issues/545.

 -- 1-based months actually trivially gives us the NED offset quarters.
 with 
    deduped as (
        select min(last_modified) as first_appearance, size from info_name group by key
    ),
    year_mo as (
        select strftime('%Y', first_appearance) as year, strftime('%m', first_appearance) as month, sum(size)/1e9 as size_gb, count(*) num_files from deduped group by year, month
    ),
    by_quarter as (
        select sum(size_gb) as size_gb, sum(num_files) as num_files, (year*12+month)/3 as quarter from year_mo group by quarter
    )
select
    -- This is some bullshit to work around 1-based months. I think it works, but the alternative is to use 0-based months earlier.
    (quarter*3-1)/12 as year,
    (quarter*3-1)%12+1 as month, 
    sum(size_gb) over win as cum_size_gb,
    sum(num_files) over win as cum_num_files
from by_quarter window win as (order by quarter);
