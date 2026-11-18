"""SMS alerts via Africa's Talking.

Credentials are read from environment variables so they are never committed:

    AT_USERNAME    Africa's Talking username ("sandbox" for the sandbox app)
    AT_API_KEY     Africa's Talking API key
    AT_RECIPIENTS  comma-separated phone numbers, e.g. "+2547XXXXXXXX,+2547YYYYYYYY"
    AT_SENDER_ID   optional sender ID
"""
import os

import africastalking

_sms = None


def _get_sms():
    """Initialise the SDK on first use (not at import time)."""
    global _sms
    if _sms is None:
        username = os.environ.get("AT_USERNAME", "sandbox")
        api_key = os.environ.get("AT_API_KEY")
        if not api_key:
            raise RuntimeError("AT_API_KEY environment variable is not set")
        africastalking.initialize(username, api_key)
        _sms = africastalking.SMS
    return _sms


def send_sms(animal="lion"):
    """Send a wildlife alert SMS. Returns True if at least one recipient was sent the message."""
    recipients = [r.strip() for r in os.environ.get("AT_RECIPIENTS", "").split(",") if r.strip()]
    if not recipients:
        print("SMS not sent: AT_RECIPIENTS environment variable is not set")
        return False

    message = f"Wildlife alert: a {animal} has been detected in your area!"
    sender_id = os.environ.get("AT_SENDER_ID") or None

    try:
        response = _get_sms().send(message, recipients, sender_id)
    except Exception as e:
        print(f"SMS failed: {e}")
        return False

    results = response.get("SMSMessageData", {}).get("Recipients", [])
    delivered = [r for r in results if r.get("status") == "Success"]
    print(f"SMS sent to {len(delivered)}/{len(recipients)} recipient(s)")
    return bool(delivered)


if __name__ == "__main__":
    send_sms()
