#!/usr/bin/env python3

import hmac
import hashlib
import base64
import time

KEY = 'Tm93IHRoYXQgeW91J3ZlIGZvdW5kIHRoaXMsIGFyZSB5b3UgcmVhZHkgdG8gam9pbiB1cz8gam9ic0B3YWxsYXBvcC5jb20=='

def get_timestamp():
    return str(int(time.time()) * 1000)

def get_signature(url, method, timestamp):
    req = url.replace('https://api.wallapop.com', '')
    msg = f'{method.upper()}|{req}|{timestamp}|'
    # print('Msg:', msg)
    sig = hmac.new(KEY.encode('utf-8'), msg.encode('utf-8'), hashlib.sha256).digest()
    return base64.b64encode(sig).decode('ascii')

def test():
    url = '/api/v3/suggesters/search'
    timestamp = '1565827270558'
    method = 'get'
    signature_good = '6iU/x0HyEqX2dzMTdv1QsTtBX4Z8tZTuHJmhzMXnxuU='

    signature = get_signature(url, method, timestamp)
    if signature == signature_good:
        print("pass")
    else:
        print("signature doesn't match")
        print("Expected:", signature_good)
        print("Obtained:", signature)
