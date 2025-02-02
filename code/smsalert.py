import africastalking

# Initialize SDK
username = "sandbox"  
api_key = "your_api_key"  # Replace with your Africa's Talking API Key
africastalking.initialize(username, api_key)

# SMS service
sms = africastalking.SMS

# Send SMS
def send_sms():
    recipients = ["+254712345678"]  # Replace with recipient phone numbers
    message = "Wildlife alert: A hyena has been detected in your area!"
    sender_id = None  # Optional. Use your sender ID if you have one
    
    try:
        response = sms.send(message, recipients, sender_id)
        print(f"Message sent! Response: {response}")
    except Exception as e:
        print(f"Error: {e}")

send_sms()
