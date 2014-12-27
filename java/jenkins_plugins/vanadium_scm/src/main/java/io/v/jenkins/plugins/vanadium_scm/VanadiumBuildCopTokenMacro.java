package io.v.jenkins.plugins.vanadium_scm;

import org.jenkinsci.plugins.tokenmacro.DataBoundTokenMacro;
import org.jenkinsci.plugins.tokenmacro.MacroEvaluationException;

import hudson.Extension;
import hudson.model.TaskListener;
import hudson.model.AbstractBuild;

import java.io.IOException;

/**
 * {@code VANADIUM_BUILDCOP} token that expands to the LDAP of the current build cop.
 */
@Extension
public class VanadiumBuildCopTokenMacro extends DataBoundTokenMacro {

  @Override
  public boolean acceptsMacroName(String macroName) {
    return macroName.equals("VANADIUM_BUILDCOP");
  }

  @Override
  public String evaluate(AbstractBuild<?, ?> context, TaskListener listener, String macroName)
      throws MacroEvaluationException, IOException, InterruptedException {
    VanadiumBuildData data = context.getAction(VanadiumBuildData.class);
    if (data == null) {
      listener.getLogger().println("No build data available");
      return "";
    }

    return data.getCurBuildCop().trim();
  }
}
