import geoip2.database

reader = geoip2.database.Reader("GeoLite2-Country_20200714/GeoLite2-Country.mmdb")

import sqlite3
import traceback
import argparse

parser = argparse.ArgumentParser()
parser.add_argument("db_file", default="confluence.db")
args_ns = parser.parse_args()

conn = sqlite3.connect(args_ns.db_file)


def ip2country(ip):
    if ip in [None, "<WebRTC>"]:
        return None
    try:
        ip = ip.rsplit(":", 1)[0].strip("[]")
        try:
            country = reader.country(ip)
        except geoip2.errors.AddressNotFoundError:
            return None
        return country.country.name
    except:
        traceback.print_exc()
        raise


conn.create_function("ip2country", 1, ip2country, deterministic=True)
for blah in conn.execute(
    "select count(*), ip2country(remote_addr) as country, extended_v from peers where extended_v is not null and country is not null group by country, extended_v order by country, count(*) desc"
):
    print(blah)
