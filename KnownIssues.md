# Known Issues

This is a list of know issues within the Skyhook Operator and details the work-around, how to recreate it, a work-around, and a fix if there is one.

### v0.5.0: Uninstalling breaks when theres an invalid image

Problem: If a package has an invalid image that doesn't exist or can't be pulled, and then they fix that image and decrease the version then the uninstall will be stuck with an image pull error as it will try to run that uninstall on that old invalid image.
    
How to Recreate: Take a package and put a bogus image starting an apply which will image pull back off. Then fix the image and decrease the package's version then issue will present itself.
    
Temporary Solution: For now the work-around on this if you get stuck is to keep on changing the version until it resolves. Although the faulty uninstall pod will still persist, and the only way to get rid of this is to remove that package from the node state on every node.

Potential Fix: This could be easily fixed if we could know that the uninstall is erroring, although the operator at the current moment doesn't indicate a image pull as an error in the node state. In order to fix this we need to watch events and put packages into an erroring state if they are in an image pull back off. This way we could check the node state and see that a package is uninstalling and erroring and remove the pod and it's node state accordingly.
    
### ~~v0.4.0: Configuration changes get stuck when it errors out~~ - **Fixed as of v0.5.0 Release**

~~Problem: If the first configuration run fails or you make a configuration change that causes the config, interrupt, or post-interrupt steps to fail then you will get stuck as all configuration changes are queued up until the package completes on the node. This means that if you make a typo which causes the config steps to fail you will be stuck.~~
    
~~How to Recreate: Create a breaking change in the configuration which causes the config steps to error out and the problem will present itself.~~
    
~~Temporary Solution: For now the only way to fix this is to change the version of the package or to change the name.~~

~~Potential Fix: In order to fix this the queuing logic for configuration changes needs to be changed or the current logic needs to account for errors and allow config changes to go through in that instance.~~
