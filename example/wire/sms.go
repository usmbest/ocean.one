package wire

import "github.com/usmbest/ocean.one/example/config"

const (
	SMSProviderTelesign = "telesign"
	SMSProviderTwilio   = "twilio"
)

func SendVerificationCodeByPhone(provider, phone, code string) error {
	if !config.SMSDeliveryEnabled {
		return nil
	}
	switch provider {
	case SMSProviderTwilio:
		if err := TwilioSendVerificationCode(phone, code); err != nil {
			return TelesignSendVerificationCode(phone, code)
		}
	default:
		if err := TelesignSendVerificationCode(phone, code); err != nil {
			return TwilioSendVerificationCode(phone, code)
		}
	}
	return nil
}
