sp := $(subst S, ,S)

test:
	echo $(sort foo bar lose)
	echo $(sort foo bar aaaa)
	echo $(sort foo bar lose lose foo bar bar)
	echo $(sort baz bar)
	echo $(sort single)
	echo $(sort $(sp)foo$(sp))
	echo $(sort )
	echo $(sort device/sample/products/AndroidProducts.mk device/moto/shamu/AndroidProducts.mk device/asus/fugu/AndroidProducts.mk device/asus/deb/AndroidProducts.mk device/asus/flo/AndroidProducts.mk device/generic/arm64/AndroidProducts.mk device/generic/qemu/AndroidProducts.mk device/generic/mini-emulator-x86_64/AndroidProducts.mk device/generic/x86/AndroidProducts.mk device/generic/mips/AndroidProducts.mk device/generic/mini-emulator-x86/AndroidProducts.mk device/generic/mini-emulator-mips/AndroidProducts.mk device/generic/mini-emulator-arm64/AndroidProducts.mk device/generic/mini-emulator-armv7-a-neon/AndroidProducts.mk device/generic/x86_64/AndroidProducts.mk device/generic/armv7-a-neon/AndroidProducts.mk device/htc/flounder/AndroidProducts.mk device/lge/bullhead/AndroidProducts.mk device/lge/hammerhead/AndroidProducts.mk device/huawei/angler/AndroidProducts.mk)
	echo $(sort cpplint-art-phony libart libartd libgabi++ libopenjdkjvm libopenjdkjvmd libart)
