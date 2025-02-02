from twilio.rest import Client

# Twilio credentials 
account_sid = 'ACc87187ba9097d7d7463e09bba88e0e51'
auth_token = '44401198fd0aa7cef05ac3e2c5cfe6ba'
twilio_number = '+17178768591'  
recipient_number = '+254111608403'  

# Initialize Twilio client
client = Client(account_sid, auth_token)

# Message content
message_body = "Alert: A lion has been detected in your area. Please stay safe!"

try:
    # Send SMS
    message = client.messages.create(
        body=message_body,
        from_=twilio_number,
        to=recipient_number
    )
    print(f"Message sent successfully! SID: {message.sid}")
except Exception as e:
    print(f"Failed to send message: {e}")
